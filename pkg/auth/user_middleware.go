package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"suse-ai-up/pkg/clients"
	"suse-ai-up/pkg/models"
)

// UserAuthService handles user authentication
type UserAuthService struct {
	UserStore    clients.UserStore
	TokenManager *TokenManager
	Config       *models.UserAuthConfig
}

// NewUserAuthService creates a new user authentication service
func NewUserAuthService(userStore clients.UserStore, tokenManager *TokenManager, config *models.UserAuthConfig) *UserAuthService {
	return &UserAuthService{
		UserStore:    userStore,
		TokenManager: tokenManager,
		Config:       config,
	}
}

// HashPassword hashes a password using bcrypt
func (uas *UserAuthService) HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// ValidatePassword checks if a password matches the hash
func (uas *UserAuthService) ValidatePassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// CreateUser creates a new user with authentication
func (uas *UserAuthService) CreateUser(ctx context.Context, user models.User, password string) error {
	// Hash password if local auth
	if user.AuthProvider == string(models.UserAuthProviderLocal) && password != "" {
		hash, err := uas.HashPassword(password)
		if err != nil {
			return fmt.Errorf("failed to hash password: %w", err)
		}
		user.PasswordHash = hash
		now := time.Now().UTC()
		user.PasswordChangedAt = &now
	}

	return uas.UserStore.Create(ctx, user)
}

// AuthenticateUser authenticates a user with credentials
func (uas *UserAuthService) AuthenticateUser(ctx context.Context, userID, password string) (*models.User, error) {
	user, err := uas.UserStore.Authenticate(ctx, userID, password)
	if err != nil {
		return nil, err
	}

	// Update last login
	now := time.Now().UTC()
	user.LastLoginAt = &now
	err = uas.UserStore.Update(ctx, *user)
	if err != nil {
		// Don't fail auth if update fails
		fmt.Printf("Warning: Failed to update last login for user %s: %v\n", userID, err)
	}

	return user, nil
}

// AuthenticateExternalUser authenticates or creates a user from external provider
func (uas *UserAuthService) AuthenticateExternalUser(ctx context.Context, provider models.UserAuthProvider, externalID, email, name string, groups []string) (*models.User, error) {
	// Try to find existing user
	user, err := uas.UserStore.GetByExternalID(ctx, string(provider), externalID)
	if err == nil {
		// Update user info and last login
		user.Name = name
		user.Email = email
		user.ProviderGroups = groups
		now := time.Now().UTC()
		user.LastLoginAt = &now
		err = uas.UserStore.Update(ctx, *user)
		if err != nil {
			return nil, fmt.Errorf("failed to update user: %w", err)
		}
		return user, nil
	}

	// Create new user
	userID := uas.generateUserID(provider, externalID)
	user = &models.User{
		ID:             userID,
		Name:           name,
		Email:          email,
		AuthProvider:   string(provider),
		ExternalID:     externalID,
		ProviderGroups: groups,
		Groups:         uas.mapExternalGroupsToLocal(provider, groups),
	}

	err = uas.UserStore.Create(ctx, *user)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return user, nil
}

// GenerateJWT generates a JWT token for a user
func (uas *UserAuthService) GenerateJWT(user *models.User) (*models.AuthToken, error) {
	claims := jwt.MapClaims{
		"sub":      user.ID,
		"email":    user.Email,
		"name":     user.Name,
		"provider": user.AuthProvider,
		"groups":   user.Groups,
		"iat":      time.Now().Unix(),
		"exp":      time.Now().Add(24 * time.Hour).Unix(), // 24 hours
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signedToken, err := token.SignedString(uas.TokenManager.GetPrivateKey())
	if err != nil {
		return nil, fmt.Errorf("failed to sign token: %w", err)
	}

	return &models.AuthToken{
		Token:     signedToken,
		TokenType: "Bearer",
		ExpiresAt: time.Now().Add(24 * time.Hour),
		UserID:    user.ID,
		Provider:  user.AuthProvider,
	}, nil
}

// ValidateJWT validates a JWT token and returns the user
func (uas *UserAuthService) ValidateJWT(tokenString string) (*models.User, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return uas.TokenManager.GetPublicKey(), nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		userID, ok := claims["sub"].(string)
		if !ok {
			return nil, fmt.Errorf("invalid token: missing subject")
		}

		user, err := uas.UserStore.Get(context.Background(), userID)
		if err != nil {
			return nil, fmt.Errorf("user not found: %w", err)
		}

		return user, nil
	}

	return nil, fmt.Errorf("invalid token")
}

// generateUserID generates a unique user ID for external users
func (uas *UserAuthService) generateUserID(provider models.UserAuthProvider, externalID string) string {
	return fmt.Sprintf("%s_%s", provider, strings.ReplaceAll(externalID, "-", "_"))
}

// mapExternalGroupsToLocal maps external groups to local groups
func (uas *UserAuthService) mapExternalGroupsToLocal(provider models.UserAuthProvider, externalGroups []string) []string {
	localGroups := []string{"mcp-users"} // Default group

	switch provider {
	case models.UserAuthProviderGitHub:
		if uas.Config.GitHub != nil {
			for _, team := range uas.Config.GitHub.AdminTeams {
				for _, extGroup := range externalGroups {
					if strings.Contains(extGroup, team) {
						localGroups = append(localGroups, "mcp-admins")
						break
					}
				}
			}
		}
	case models.UserAuthProviderRancher:
		if uas.Config.Rancher != nil {
			for _, adminGroup := range uas.Config.Rancher.AdminGroups {
				for _, extGroup := range externalGroups {
					if extGroup == adminGroup {
						localGroups = append(localGroups, "mcp-admins")
						break
					}
				}
			}
		}
	}

	return localGroups
}

// UserAuthMiddleware creates Gin middleware for user authentication
func UserAuthMiddleware(authService *UserAuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if dev mode is enabled
		if authService.Config.DevMode {
			// In dev mode, allow X-User-ID header to bypass auth
			userID := c.GetHeader("X-User-ID")
			if userID != "" {
				user, err := authService.UserStore.Get(c.Request.Context(), userID)
				if err == nil {
					c.Set("user", user)
					c.Next()
					return
				}
			}
			// Check if request comes from development origins (allow anonymous access)
			origin := c.GetHeader("Origin")
			if origin != "" && (strings.Contains(origin, "localhost") ||
				strings.Contains(origin, "127.0.0.1") ||
				strings.Contains(origin, "192.168.") ||
				strings.Contains(origin, "10.")) {
				// Allow anonymous access from development origins with admin permissions
				c.Set("user", &models.User{ID: "dev-admin", Name: "Dev Admin", AuthProvider: "dev"})
				c.Request.Header.Set("X-User-ID", "dev-admin")
				c.Next()
				return
			}
			// If no valid user, set anonymous with admin permissions for dev mode
			c.Set("user", &models.User{ID: "dev-admin", Name: "Dev Admin", AuthProvider: "dev"})
			c.Request.Header.Set("X-User-ID", "dev-admin")
			c.Next()
			return
		}

		// Extract token from Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization required"})
			c.Abort()
			return
		}

		tokenString, err := ExtractTokenFromHeader(authHeader)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			c.Abort()
			return
		}

		// Validate token
		user, err := authService.ValidateJWT(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			c.Abort()
			return
		}

		c.Set("user", user)
		c.Next()
	}
}

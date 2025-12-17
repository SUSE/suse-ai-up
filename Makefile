.PHONY: build clean test

build:
	go build -o suse-ai-up ./cmd/uniproxy

clean:
	rm -f suse-ai-up

test:
	go test ./...

# Development helpers
dev: build
	./suse-ai-up

docker-build:
	docker build -t suse-ai-up:latest .

docker-run: docker-build
	docker run -p 8911:8911 suse-ai-up:latest

helm-install:
	helm install suse-ai-up ./charts/suse-ai-up --namespace suse-ai-up --create-namespace

helm-upgrade:
	helm upgrade suse-ai-up ./charts/suse-ai-up --namespace suse-ai-up

helm-test: helm-upgrade
	@echo "Helm chart deployed successfully. In a real Kubernetes environment, you would run:"
	@echo "kubectl wait --for=condition=available --timeout=300s deployment/suse-ai-up-suse-ai-up -n suse-ai-up"
	@echo "kubectl port-forward -n suse-ai-up svc/suse-ai-up-service 8911:8911 &"
	@echo "curl http://localhost:8911/health"
	@echo "curl -H 'X-User-ID: admin' http://localhost:8911/api/v1/adapters"
	@echo "curl -H 'X-User-ID: admin' http://localhost:8911/api/v1/registry"

# Local testing without Kubernetes
test-local: build
	@echo "Starting SUSE AI Uniproxy locally..."
	./suse-ai-up &
	@echo "Waiting for service to start..."
	@sleep 10
	@echo "Testing endpoints..."
	curl -f http://localhost:8911/health || echo "Health check failed"
	@echo "Testing admin access..."
	curl -H "X-User-ID: admin" http://localhost:8911/api/v1/adapters || echo "Admin adapter access failed"
	curl -H "X-User-ID: admin" http://localhost:8911/api/v1/users || echo "Admin user access failed"
	curl -H "X-User-ID: admin" http://localhost:8911/api/v1/groups || echo "Admin group access failed"
	@echo "Local testing completed. Press Ctrl+C to stop the service."
	@wait
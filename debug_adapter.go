package main

import (
	"encoding/json"
	"fmt"
	"suse-ai-up/pkg/models"
	"time"
)

func main() {
	data := models.AdapterData{
		Name:                 "test",
		ConnectionType:       models.ConnectionTypeStreamableHttp,
		RemoteUrl:            "http://example.com",
		EnvironmentVariables: map[string]string{},
	}

	adapter := &models.AdapterResource{}
	adapter.Create(data, "system", time.Now())

	fmt.Printf("Adapter created: %+v\n", adapter)

	jsonBytes, err := json.Marshal(adapter)
	if err != nil {
		fmt.Printf("Error marshaling: %v\n", err)
		return
	}

	fmt.Printf("JSON: %s\n", string(jsonBytes))
}

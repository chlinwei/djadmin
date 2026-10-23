package config

import "testing"

func TestDatabaseRolesDoNotRequireRabbitMQ(t *testing.T) {
	configuration := Config{MySQLDSN: "test-dsn"}
	for _, role := range []string{"scheduler", "worker"} {
		t.Run(role, func(t *testing.T) {
			if err := configuration.Validate(role); err != nil {
				t.Fatalf("database-only role requires RabbitMQ: %v", err)
			}
		})
	}
}

func TestAPIStillRequiresRabbitMQ(t *testing.T) {
	configuration := Config{MySQLDSN: "test-dsn", JWTSecret: "test"}
	if err := configuration.Validate("api"); err == nil {
		t.Fatal("api must require RabbitMQ for log-collection batches")
	}
	configuration.RabbitMQURL = "test-url"
	if err := configuration.Validate("api"); err != nil {
		t.Fatal(err)
	}
}

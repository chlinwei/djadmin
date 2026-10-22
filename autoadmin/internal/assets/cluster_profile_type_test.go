package assets

import (
	"context"
	"errors"
	"testing"
)

func TestCreateProfileRejectsBuiltinAndRemovedTypes(t *testing.T) {
	service := &Service{}
	for _, value := range []struct {
		profileType string
		clusterType string
	}{
		{"builtin", "ha"},
		{"builtin", "mysql"},
		{"custom", "redis"},
		{"custom", "elasticsearch"},
		{"custom", "nacos"},
	} {
		t.Run(value.profileType+"/"+value.clusterType, func(t *testing.T) {
			_, err := service.CreateProfile(context.Background(), ClusterProfileInput{
				Name: "Model", Code: "model", ProfileType: value.profileType, ClusterType: value.clusterType,
			})
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("expected invalid model type, got %v", err)
			}
		})
	}
}

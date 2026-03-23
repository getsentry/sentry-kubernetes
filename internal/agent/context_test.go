package agent

import (
	"context"
	"testing"

	"k8s.io/client-go/kubernetes/fake"
)

func TestSetAndGetClientsetFromContext(t *testing.T) {
	ctx := context.Background()
	fakeClientset := fake.NewSimpleClientset()

	ctx = setClientsetOnContext(ctx, fakeClientset)

	clientset, err := getClientsetFromContext(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if clientset != fakeClientset {
		t.Error("retrieved clientset does not match the one that was set")
	}
}

func TestGetClientsetFromContextMissing(t *testing.T) {
	ctx := context.Background()

	_, err := getClientsetFromContext(ctx)
	if err == nil {
		t.Error("expected error when clientset is not on context")
	}
}

func TestGetClientsetFromContextWrongType(t *testing.T) {
	ctx := context.WithValue(context.Background(), clientsetCtxKey{}, "not-a-clientset")

	_, err := getClientsetFromContext(ctx)
	if err == nil {
		t.Error("expected error when context value is wrong type")
	}
}

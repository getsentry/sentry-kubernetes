package main

import (
	"context"
	"fmt"

	"k8s.io/client-go/kubernetes"
)

type clientsetCtxKey struct{}

// Note: we do not use kubernetes.Clientset type directly, to enable mocking via "k8s.io/client-go/kubernetes/fake"
// package. A type alias is introduced to avoid confusion: "kubernetes.Interface" is actually a clientset.Interface, and
// not a generic interface for Kubernetes resources.
type ClientsetInterface = kubernetes.Interface

func setClientsetOnContext(ctx context.Context, clientset ClientsetInterface) context.Context {
	return context.WithValue(ctx, clientsetCtxKey{}, clientset)
}

func getClientsetFromContext(ctx context.Context) (ClientsetInterface, error) {
	val := ctx.Value(clientsetCtxKey{})
	if val == nil {
		return nil, fmt.Errorf("no clientset present on context")
	}
	if clientset, ok := val.(ClientsetInterface); ok {
		return clientset, nil
	} else {
		return nil, fmt.Errorf("cannot convert clientset value from context")
	}
}

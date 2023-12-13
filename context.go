package main

import (
	"context"
	"fmt"

	"k8s.io/client-go/kubernetes"
)

type clientsetCtxKey struct{}

func setClientsetOnContext(ctx context.Context, clientset kubernetes.Interface) context.Context {
	return context.WithValue(ctx, clientsetCtxKey{}, clientset)
}

func getClientsetFromContext(ctx context.Context) (kubernetes.Interface, error) {
	val := ctx.Value(clientsetCtxKey{})
	if val == nil {
		return nil, fmt.Errorf("no clientset present on context")
	}
	if clientset, ok := val.(kubernetes.Interface); ok {
		return clientset, nil
	} else {
		return nil, fmt.Errorf("cannot convert clientset value from context")
	}
}

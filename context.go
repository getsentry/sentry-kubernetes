package main

import (
	"context"
	"fmt"

	"k8s.io/client-go/kubernetes"
)

type clientsetCtxKey struct{}

func setClientsetOnContext(ctx context.Context, clientset *kubernetes.Clientset) context.Context {
	return context.WithValue(ctx, clientsetCtxKey{}, clientset)
}

func getClientsetFromContext(ctx context.Context) (*kubernetes.Clientset, error) {
	val := ctx.Value(clientsetCtxKey{})
	if val == nil {
		return nil, fmt.Errorf("no clientset present on context")
	}
	if clientset, ok := val.(*kubernetes.Clientset); ok {
		return clientset, nil
	} else {
		return nil, fmt.Errorf("cannot convert clientset value from context")
	}
}

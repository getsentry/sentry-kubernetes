package main

import (
	"container/ring"
	"sync"

	v1 "k8s.io/api/core/v1"
)

// TODO: check if we even need this if we use NewSharedInformerFactory for
// watching events.

const bufferSize = 1000

var mu sync.RWMutex

// TODO: we could have one buffer per namespace
var eventBuffer *ring.Ring = ring.New(bufferSize)

func addEventToBuffer(event *v1.Event) {
	mu.Lock()
	defer mu.Unlock()

	eventBuffer.Value = event.DeepCopy()
	eventBuffer = eventBuffer.Next()
}

func filterEventsFromBuffer(namespace string, objectKind string, objectName string) []*v1.Event {
	mu.RLock()
	defer mu.RUnlock()

	res := make([]*v1.Event, 0, bufferSize/4)

	// FIXME: this can be optimized if the limit is provided and we walk
	// backwards.
	eventBuffer.Do(func(obj any) {
		event, ok := obj.(*v1.Event)
		if !ok {
			return
		}
		if event.Namespace != namespace ||
			event.InvolvedObject.Kind != objectKind ||
			event.InvolvedObject.Name != objectName {
			return
		}
		res = append(res, event.DeepCopy())
	})
	return res
}

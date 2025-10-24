package controller

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
)

// DynamicObject wraps a runtime.Object along with a flag indicating whether it should be created.
type DynamicObject struct {
	// Obj is the dynamic object to be added or created.
	Obj runtime.Object
	// Create indicates whether the object should be created instead of added. Refer to NewDynamicClient for details.
	Create bool
}

// NewDynamicClient creates a fake dynamic client with the given scheme and objects.
//
// This function is based on `fake.NewSimpleDynamicClient`. It builds on top to solve incorrect GVR guessing. Reference:
// https://github.com/kubernetes/client-go/blob/4953849642e8eef9957516964dad0585e0c4cf71/dynamic/fake/simple.go#L37
//
// Some GVRs may be overriden. The default behavior is to try guessing the plural version of the kind, but that guess
// may be wrong in some cases (e.g. "NodeOverlay" becomes "nodeoverlaies" instead of "nodeoverlays"). In those cases,
// the caller has to provide the correct mapping in the override map.
//
// Furthermore, if a GVR was overriden, then objects of that GVR can't be "Added" and must be "Created" instead. So, for
// those objects, set the create flag to true. Fake dynamic client uses the same fragile GVR guessing logic when adding
// objects, so we have to create them by specifying the GVR explicitly. If you try to add a NodeOverlay, then it will
// get added to "nodeoverlaies" resource, and then any subsequent Get/List on "nodeoverlays" will not find it.
// Reference: https://github.com/kubernetes/client-go/blob/4953849642e8eef9957516964dad0585e0c4cf71/testing/fixture.go#L431
func NewDynamicClient(
	t *testing.T,
	scheme *runtime.Scheme,
	override map[schema.GroupVersionResource]string,
	objects ...*DynamicObject,
) (*fake.FakeDynamicClient, error) {
	unstructuredScheme := runtime.NewScheme()

	// Register all known types as unstructured.
	for gvk := range scheme.AllKnownTypes() {
		if unstructuredScheme.Recognizes(gvk) {
			continue
		}
		if strings.HasSuffix(gvk.Kind, "List") {
			unstructuredScheme.AddKnownTypeWithName(gvk, &unstructured.UnstructuredList{})
			continue
		}
		unstructuredScheme.AddKnownTypeWithName(gvk, &unstructured.Unstructured{})
	}

	// Convert all objects to unstructured and separate those to be created vs added.
	var allObjects []runtime.Object
	var addObjects []runtime.Object
	var createObjects []*unstructured.Unstructured
	for _, obj := range objects {
		o, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj.Obj)
		require.NoError(t, err)

		u := &unstructured.Unstructured{Object: o}

		if gvk := u.GroupVersionKind(); gvk.Group == "" || gvk.Version == "" {
			require.Fail(t, "object must have GVK set")
		}

		allObjects = append(allObjects, u)

		if obj.Create {
			createObjects = append(createObjects, u)
		} else {
			addObjects = append(addObjects, u)
		}
	}

	// Register any missing types from the given objects list.
	for _, obj := range allObjects {
		gvk := obj.GetObjectKind().GroupVersionKind()
		if !unstructuredScheme.Recognizes(gvk) {
			unstructuredScheme.AddKnownTypeWithName(gvk, &unstructured.Unstructured{})
		}
		gvk.Kind += "List"
		if !unstructuredScheme.Recognizes(gvk) {
			unstructuredScheme.AddKnownTypeWithName(gvk, &unstructured.UnstructuredList{})
		}
	}

	// Create the dynamic client. Objects to be added are passed here.
	dynamicClient := fake.NewSimpleDynamicClientWithCustomListKinds(unstructuredScheme, override, addObjects...)

	// Create the objects that need to be created.
	for _, obj := range createObjects {
		gvk := obj.GroupVersionKind()

		resource := ""

		// Try to find the resource name from the override map.
		for gvr, kind := range override {
			k := strings.TrimSuffix(kind, "List")

			if gvk.Kind == k && gvk.Group == gvr.Group && gvk.Version == gvr.Version {
				resource = gvr.Resource
				break
			}
		}

		require.NotEmpty(t, resource, "Object was supposed to be created but its plural GVR was not found in the GVR overrides map. Did you forget to add it? GVK: %v", gvk)

		gvr := schema.GroupVersionResource{
			Group:    gvk.Group,
			Version:  gvk.Version,
			Resource: resource,
		}

		// Create the object.
		_, err := dynamicClient.Resource(gvr).Namespace(obj.GetNamespace()).Create(t.Context(), obj, metav1.CreateOptions{})
		require.NoError(t, err)
	}

	return dynamicClient, nil
}

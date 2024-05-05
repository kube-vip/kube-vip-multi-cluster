package payload

import (
	"bytes"
	"io"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/restmapper"
	"k8s.io/klog/v2"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const DefaultDecoderBufferSize = 500

// Take the raw data and convert into objects
func Decode(data []byte) ([]unstructured.Unstructured, error) {
	var lastErr error
	var unstructList []unstructured.Unstructured
	i := 1

	decoder := yamlutil.NewYAMLOrJSONDecoder(bytes.NewReader(data), DefaultDecoderBufferSize)
	for {
		var reqObj runtime.RawExtension
		if err := decoder.Decode(&reqObj); err != nil {
			lastErr = err
			break
		}
		klog.V(5).Infof("The section:[%d] raw content: %s", i, string(reqObj.Raw))
		if len(reqObj.Raw) == 0 {
			continue
		}

		obj, gvk, err := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme).Decode(reqObj.Raw, nil, nil)
		if err != nil {
			lastErr = errors.Wrapf(err, "serialize the section:[%d] content error", i)
			klog.Info(lastErr)
			break
		}
		klog.V(5).Infof("The section:[%d] GroupVersionKind: %#v  object: %#v", i, gvk, obj)

		unstruct, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		if err != nil {
			lastErr = errors.Wrapf(err, "serialize the section:[%d] content error", i)
			break
		}
		unstructList = append(unstructList, unstructured.Unstructured{Object: unstruct})
		i++
	}

	if lastErr != io.EOF {
		return unstructList, errors.Wrapf(lastErr, "parsing the section:[%d] content error", i)
	}

	return unstructList, nil
}

func GetRestMapping(discoveryClient *discovery.DiscoveryClient, gvk schema.GroupVersionKind) (*meta.RESTMapping, error) {
	// Create a REST mapper that tracks information about the available resources in the cluster.
	groupResources, err := restmapper.GetAPIGroupResources(discoveryClient)
	if err != nil {
		return nil, err
	}
	rm := restmapper.NewDiscoveryRESTMapper(groupResources)

	gk := schema.GroupKind{Group: gvk.Group, Kind: gvk.Kind}
	mapping, err := rm.RESTMapping(gk, gvk.Version)
	if err != nil {
		return nil, err
	}
	return mapping, nil
}

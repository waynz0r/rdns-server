package k8s

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	k8sclient "github.com/rancher/rdns-server/k8s/client"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const StorageName = "k8s"

type K8sStore struct {
	mgr k8sclient.Manager
	mux sync.Mutex

	namespace string
}

func New(mgr k8sclient.Manager, namespace string) (*K8sStore, error) {
	ns := &corev1.Namespace{}
	err := mgr.GetClient().Get(context.Background(), types.NamespacedName{
		Name: namespace,
	}, ns)
	if err != nil {
		return nil, err
	}

	return &K8sStore{
		mgr: mgr,
		mux: sync.Mutex{},

		namespace: ns.Name,
	}, nil
}

func (s *K8sStore) SetValue(name, valueType string, metadata interface{}) error {
	return s.writeValue(name, valueType, metadata, false)
}

func (s *K8sStore) UpdateValue(name, valueType string, metadata interface{}) error {
	return s.writeValue(name, valueType, metadata, true)
}

func (s *K8sStore) generateName(name, valueType string) string {
	name = fmt.Sprintf("%x", md5.Sum([]byte(name)))

	return strings.Join([]string{name, valueType}, "-")
}

func (s *K8sStore) writeValue(name, valueType string, metadata interface{}, update bool) error {
	rname := s.generateName(name, valueType)

	var create bool
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rname,
			Namespace: s.namespace,
		},
	}
	err := s.mgr.GetClient().Get(context.Background(), types.NamespacedName{
		Name:      cm.Name,
		Namespace: cm.Namespace,
	}, cm)
	if err != nil && !k8serrors.IsNotFound(err) {
		return err
	}
	if k8serrors.IsNotFound(err) {
		create = true
	}

	if cm.Annotations == nil {
		cm.Annotations = make(map[string]string)
	}
	cm.Annotations["rdns-name"] = name

	if cm.Labels == nil {
		cm.Labels = make(map[string]string)
	}
	cm.Labels["rdns-value-type"] = valueType
	cm.Labels["rnds-value"] = "true"

	j, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}
	cm.Data["data"] = string(j)

	if create {
		logrus.Debugf("create configmap: %s/%s", cm.Namespace, cm.Name)
		return s.mgr.GetClient().Create(context.Background(), cm)
	}

	logrus.Debugf("update configmap: %s/%s", cm.Namespace, cm.Name)
	return s.mgr.GetClient().Update(context.Background(), cm)
}

func (s *K8sStore) DeleteValue(name, valueType string) error {
	s.mux.Lock()
	defer s.mux.Unlock()

	rname := s.generateName(name, valueType)
	cm := &corev1.ConfigMap{}
	err := s.mgr.GetClient().Get(context.Background(), types.NamespacedName{
		Name:      rname,
		Namespace: s.namespace,
	}, cm)
	if k8serrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	return s.mgr.GetClient().Delete(context.Background(), cm)
}

func (s *K8sStore) ListValues(valueType string) ([]string, error) {
	cms := &corev1.ConfigMapList{}
	err := s.mgr.GetClient().List(context.Background(), cms, client.InNamespace(s.namespace), client.MatchingLabels(map[string]string{
		"rdns-value-type": valueType,
	}))

	if err != nil {
		return nil, err
	}

	names := make([]string, len(cms.Items))
	for i, cm := range cms.Items {
		names[i] = cm.Annotations["rdns-name"]
	}

	return names, nil
}

func (s *K8sStore) GetValue(name, valueType string, metadata interface{}) (string, error) {
	rname := s.generateName(name, valueType)

	cm := &corev1.ConfigMap{}
	err := s.mgr.GetClient().Get(context.Background(), types.NamespacedName{
		Name:      rname,
		Namespace: s.namespace,
	}, cm)
	if err != nil && !k8serrors.IsNotFound(err) {
		return "", err
	}

	if k8serrors.IsNotFound(err) {
		return "", nil
	}

	err = json.Unmarshal([]byte(cm.Data["data"]), &metadata)
	if err != nil {
		return "", err
	}

	return cm.Data["data"], nil
}

func (s *K8sStore) GetExpiredValues(valueType string, t *time.Time) ([]string, error) {
	expired := make([]string, 0)

	names, err := s.ListValues(valueType)
	if err != nil {
		return nil, err
	}

	for _, name := range names {
		var record struct{ CreatedOn int64 }
		_, err := s.GetValue(name, valueType, &record)
		if err != nil {
			return expired, err
		}
		if record.CreatedOn < t.UnixNano() {
			expired = append(expired, name)
		}
	}

	return expired, nil
}

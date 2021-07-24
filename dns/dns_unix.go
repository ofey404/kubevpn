package dns

import (
	"context"
	"github.com/pkg/errors"
	"io/fs"
	"io/ioutil"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"path/filepath"
)

func Dns(clientset *kubernetes.Clientset) error {
	var dnsIP string
	var err error
	if dnsIP, err = GetDNSIp(clientset); err != nil {
		return err
	}
	filename := filepath.Join("etc", "resolver", "local")
	fileContent := "nameserver " + dnsIP
	return ioutil.WriteFile(filename, []byte(fileContent), fs.ModePerm)
}

func GetDNSIp(clientset *kubernetes.Clientset) (string, error) {
	serviceList, err := clientset.CoreV1().Services(v1.NamespaceSystem).List(context.Background(), v1.ListOptions{
		LabelSelector: fields.OneTermEqualSelector("k8s-app", "kube-dns").String(),
	})
	if err != nil {
		return "", err
	}
	if len(serviceList.Items) == 0 {
		return "", errors.New("Not found kube-dns")
	}
	return serviceList.Items[0].Spec.ClusterIP, nil
}

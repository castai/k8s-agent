//go:generate mockgen -destination ./mock/version.go . Interface
package version

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes"
)

type Interface interface {
	Full() string
	MinorInt() int
}

func Get(log logrus.FieldLogger, clientset kubernetes.Interface) (Interface, error) {
	cs, ok := clientset.(*kubernetes.Clientset)
	if !ok {
		return nil, fmt.Errorf("expected clientset to be of type *kubernetes.Clientset but was %T", clientset)
	}

	sv, err := cs.ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("getting server version: %w", err)
	}

	log.Infof("kubernetes version %s.%s", sv.Major, sv.Minor)

	m, err := strconv.Atoi(regexp.MustCompile(`^(\d+)`).FindString(sv.Minor))
	if err != nil {
		return nil, fmt.Errorf("parsing minor version: %w", err)
	}

	return &Version{v: sv, m: m}, nil
}

type Version struct {
	v *version.Info
	m int
}

func (v *Version) Full() string {
	if v.v.GitVersion != "" {
		return strings.TrimPrefix(v.v.GitVersion, "v")
	}
	// We should rarely, if ever, have empty GitVersion, but this is just in case
	return v.v.Major + "." + v.v.Minor + ".0"
}

func (v *Version) MinorInt() int {
	return v.m
}

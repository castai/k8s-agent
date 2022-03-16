package config

import (
	"reflect"
	"strings"

	"github.com/spf13/viper"
)

// https://github.com/spf13/viper/issues/188#issuecomment-399884438
func bindEnvs(iface interface{}, parts ...string) {
	ifv := reflect.ValueOf(iface)
	ift := reflect.TypeOf(iface)
	for i := 0; i < ift.NumField(); i++ {
		v := ifv.Field(i)
		t := ift.Field(i)
		tv, ok := t.Tag.Lookup("mapstructure")
		if !ok {
			continue
		}
		// Resolve pointers to strcuts
		if v.Kind() == reflect.Ptr && v.Type().Elem().Kind() == reflect.Struct {
			v = reflect.New(v.Type().Elem()).Elem()
		}
		switch v.Kind() {
		case reflect.Struct:
			bindEnvs(v.Interface(), append(parts, tv)...)
		default:
			_ = viper.BindEnv(strings.Join(append(parts, tv), "."))
		}
	}
}

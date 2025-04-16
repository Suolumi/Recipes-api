package utils

import (
	"fmt"
	"github.com/joho/godotenv"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

func parseEnv(cfg interface{}, path string) error {
	val := reflect.ValueOf(cfg).Elem()
	typ := val.Type()

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := typ.Field(i)

		// Check if the field is a pointer to a struct
		if fieldType.Type.Kind() == reflect.Ptr && fieldType.Type.Elem().Kind() == reflect.Struct {
			// Recursively load the nested struct fields
			err := parseEnv(field.Interface(), path+fieldType.Name+"_")
			if err != nil {
				return err
			}
		} else {
			// Load environment variables for non-struct fields
			envName := strings.ToUpper(path + fieldType.Name)
			envValue := os.Getenv(envName)

			if envValue != "" && field.CanSet() {
				err := setField(field, envValue)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func setField(field reflect.Value, envValue string) error {
	switch field.Interface().(type) {
	case time.Duration:
		return setDuration(field, envValue)
	}

	switch field.Kind() {
	case reflect.Int:
		return setInt(field, envValue)
	case reflect.String:
		field.SetString(envValue)
	case reflect.Float32, reflect.Float64:
		return setFloat(field, envValue)
	default:
		return fmt.Errorf("unsupported field type: %s", field.Type().String())
	}

	return nil
}

func setInt(field reflect.Value, envValue string) error {
	intValue, err := strconv.Atoi(envValue)
	if err != nil {
		return err
	}
	field.SetInt(int64(intValue))
	return nil
}

func setFloat(field reflect.Value, envValue string) error {
	bitSize := 32
	if field.Kind() == reflect.Float64 {
		bitSize = 64
	}
	floatValue, err := strconv.ParseFloat(envValue, bitSize)
	if err != nil {
		return err
	}
	field.SetFloat(floatValue)
	return nil
}

func setDuration(field reflect.Value, envValue string) error {
	durationValue, err := time.ParseDuration(envValue)
	if err != nil {
		return err
	}
	field.Set(reflect.ValueOf(durationValue))
	return nil
}

func LoadConfigFromEnv(cfg interface{}, prefix ...string) error {
	_ = godotenv.Load()
	path := strings.Join(prefix, "_") + "_"

	return parseEnv(cfg, path)
}

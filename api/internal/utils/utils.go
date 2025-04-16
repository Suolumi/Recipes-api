package utils

import (
	"fmt"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
	"reflect"
	"regexp"
	"strings"
)

func IsEmailValid(email string) bool {
	pattern := `^(([^<>()[\]\\.,;:\s@"]+(\.[^<>()[\]\\.,;:\s@"]+)*)|.(".+"))@((\[[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}])|(([a-zA-Z\-0-9]+\.)+[a-zA-Z]{2,}))$`
	regex := regexp.MustCompile(pattern)
	return regex.MatchString(email)
}

// IsPasswordValid regex doesn't compile here because of RE2 format instead of PCRE
func IsPasswordValid(password string) bool {
	length, numbers, specials := 10, 1, 1

	for _, c := range password {
		if c >= '0' && c <= '9' {
			numbers--
		} else if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
			specials--
		}
	}
	return len(password) >= length && numbers <= 0 && specials <= 0
}

func HashPassword(password string) (string, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return "", err
	}
	return string(hashedPassword), nil
}

func ComparePasswords(hashed string, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashed), []byte(password))
}

func BindQuery(c echo.Context, i interface{}) error {
	val := reflect.ValueOf(i).Elem()
	typ := val.Type()

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := typ.Field(i)
		// Split to remove ,omitempty
		queryName := strings.Split(fieldType.Tag.Get("query"), ",")[0]
		queryValue := c.QueryParam(queryName)

		if queryValue != "" && field.CanSet() {
			err := setField(field, queryValue)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func CopyStruct(dest interface{}, src interface{}) {
	srcVal := reflect.ValueOf(src).Elem()
	destVal := reflect.ValueOf(dest).Elem()

	if !srcVal.IsValid() || !destVal.IsValid() {
		return
	}

	typ := destVal.Type()
	for i := 0; i < destVal.NumField(); i++ {
		field := destVal.Field(i)
		fieldType := typ.Field(i)
		value := srcVal.FieldByName(fieldType.Name)

		// Check if the field is a pointer to a struct
		if value.IsValid() && field.IsValid() &&
			fieldType.Type.Kind() == reflect.Ptr && fieldType.Type.Elem().Kind() == reflect.Struct &&
			value.Type().Kind() == reflect.Ptr && value.Elem().Kind() == reflect.Struct {
			if field.IsZero() {
				field.Set(reflect.New(field.Type().Elem()))
			}
			CopyStruct(field.Interface(), value.Interface())
		} else if field.CanSet() && value.IsValid() {
			field.Set(value)
		}
	}
}

func DupStruct[T interface{}](src interface{}) T {
	var dest T
	CopyStruct(&dest, src)
	return dest
}

func LogError(message string, err error) {
	fmt.Println(message + ": " + err.Error())
}

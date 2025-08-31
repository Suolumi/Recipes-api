package images_manager

import (
	"fmt"
	"golang.org/x/exp/slices"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
)

func Save(formFile *multipart.FileHeader, saveDir, fileName string) (rerr error) {
	filePath := filepath.Join(saveDir, fileName)

	reader, err := formFile.Open()
	if err != nil {
		return err
	}
	defer func(reader multipart.File) {
		rerr = reader.Close()
	}(reader)

	writer, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, os.ModePerm)
	if err != nil {
		return err
	}
	defer func(writer *os.File) {
		rerr = writer.Close()
	}(writer)

	_, err = io.Copy(writer, reader)
	if err != nil {
		return err
	}
	return nil
}

func Remove(saveDir, fileName string) error {
	filePath := filepath.Join(saveDir, fileName)

	return os.Remove(filePath)
}

func CheckImage(formFile *multipart.FileHeader) error {
	supportedExtensions := []string{".jpg", ".jpeg", ".png"}

	if !slices.Contains(supportedExtensions, filepath.Ext(formFile.Filename)) {
		return fmt.Errorf("Valid image formats are: " + strings.Join(supportedExtensions, ", "))
	}

	maxFileSize := int64(8388608) // 8 MB in bytes
	if formFile.Size > maxFileSize {
		return fmt.Errorf("file size exceeds 8 MB limit")
	}

	return nil
}

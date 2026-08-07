package images_manager

import (
	"bytes"
	"fmt"
	"github.com/disintegration/imaging"
	"golang.org/x/exp/slices"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	MaxRecipeImagePixels = 40_000_000
	MaxRecipeImageAxis   = 16_384
)

type NormalizedImage struct {
	Data      []byte
	Extension string
	MediaType string
}

func Save(formFile *multipart.FileHeader, saveDir, fileName string) (rerr error) {
	if formFile == nil {
		return fmt.Errorf("image is required")
	}
	filePath := filepath.Join(saveDir, filepath.Base(fileName))
	if err := os.MkdirAll(saveDir, 0o700); err != nil {
		return err
	}

	reader, err := formFile.Open()
	if err != nil {
		return err
	}
	defer reader.Close()

	writer, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err = io.Copy(writer, reader); err != nil {
		_ = writer.Close()
		return err
	}
	return writer.Close()
}

func Remove(saveDir, fileName string) error {
	if strings.TrimSpace(fileName) == "" {
		return nil
	}
	filePath := filepath.Join(saveDir, filepath.Base(fileName))

	return os.Remove(filePath)
}

// NormalizeRecipeImage validates an uploaded recipe image using its bytes,
// applies EXIF orientation, and re-encodes it to strip metadata.
func NormalizeRecipeImage(data []byte, filename, declaredMediaType string) (NormalizedImage, error) {
	if len(data) == 0 {
		return NormalizedImage{}, fmt.Errorf("image is empty")
	}

	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return NormalizedImage{}, fmt.Errorf("invalid image: %w", err)
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > MaxRecipeImageAxis || config.Height > MaxRecipeImageAxis ||
		int64(config.Width)*int64(config.Height) > MaxRecipeImagePixels {
		return NormalizedImage{}, fmt.Errorf("image dimensions exceed the supported limit")
	}

	detectedMediaType := strings.Split(http.DetectContentType(data), ";")[0]
	var outputFormat imaging.Format
	var extension, mediaType string
	switch format {
	case "jpeg":
		outputFormat, extension, mediaType = imaging.JPEG, ".jpg", "image/jpeg"
	case "png":
		outputFormat, extension, mediaType = imaging.PNG, ".png", "image/png"
	default:
		return NormalizedImage{}, fmt.Errorf("supported image formats are JPEG and PNG")
	}
	if detectedMediaType != mediaType {
		return NormalizedImage{}, fmt.Errorf("image content does not match a supported media type")
	}
	if declaredMediaType != "" && strings.Split(strings.ToLower(declaredMediaType), ";")[0] != mediaType {
		return NormalizedImage{}, fmt.Errorf("declared media type does not match image content")
	}
	if filename != "" {
		ext := strings.ToLower(filepath.Ext(filename))
		if (format == "jpeg" && ext != ".jpg" && ext != ".jpeg") || (format == "png" && ext != ".png") {
			return NormalizedImage{}, fmt.Errorf("filename extension does not match image content")
		}
	}

	decoded, err := imaging.Decode(bytes.NewReader(data), imaging.AutoOrientation(true))
	if err != nil {
		return NormalizedImage{}, fmt.Errorf("decode image: %w", err)
	}
	var normalized bytes.Buffer
	options := []imaging.EncodeOption{}
	if outputFormat == imaging.JPEG {
		options = append(options, imaging.JPEGQuality(90))
	}
	if err := imaging.Encode(&normalized, decoded, outputFormat, options...); err != nil {
		return NormalizedImage{}, fmt.Errorf("encode image: %w", err)
	}
	return NormalizedImage{Data: normalized.Bytes(), Extension: extension, MediaType: mediaType}, nil
}

func SaveBytes(saveDir, fileName string, data []byte) error {
	if err := os.MkdirAll(saveDir, 0o700); err != nil {
		return err
	}
	filePath := filepath.Join(saveDir, filepath.Base(fileName))
	return os.WriteFile(filePath, data, 0o600)
}

func NormalizeMultipartImage(formFile *multipart.FileHeader, maxBytes int64) (NormalizedImage, error) {
	if formFile == nil {
		return NormalizedImage{}, fmt.Errorf("image is required")
	}
	if maxBytes > 0 && formFile.Size > maxBytes {
		return NormalizedImage{}, fmt.Errorf("file size exceeds %d bytes", maxBytes)
	}
	file, err := formFile.Open()
	if err != nil {
		return NormalizedImage{}, err
	}
	defer file.Close()
	reader := io.Reader(file)
	if maxBytes > 0 {
		reader = io.LimitReader(file, maxBytes+1)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return NormalizedImage{}, err
	}
	if maxBytes > 0 && int64(len(data)) > maxBytes {
		return NormalizedImage{}, fmt.Errorf("file size exceeds %d bytes", maxBytes)
	}
	return NormalizeRecipeImage(data, formFile.Filename, formFile.Header.Get("Content-Type"))
}

func CheckImage(formFile *multipart.FileHeader) error {
	if formFile == nil {
		return fmt.Errorf("image is required")
	}
	if !slices.Contains([]string{".jpg", ".jpeg", ".png"}, strings.ToLower(filepath.Ext(formFile.Filename))) {
		return fmt.Errorf("valid image formats are: .jpg, .jpeg, .png")
	}
	_, err := NormalizeMultipartImage(formFile, 8<<20)
	return err
}

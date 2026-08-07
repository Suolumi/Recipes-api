package images_manager

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func encodedImage(t *testing.T, format string, width, height int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	img.Set(0, 0, color.NRGBA{R: 255, A: 255})
	var output bytes.Buffer
	var err error
	if format == "jpeg" {
		err = jpeg.Encode(&output, img, nil)
	} else {
		err = png.Encode(&output, img)
	}
	require.NoError(t, err)
	return output.Bytes()
}

func TestNormalizeRecipeImage(t *testing.T) {
	t.Run("normalizes jpeg", func(t *testing.T) {
		result, err := NormalizeRecipeImage(encodedImage(t, "jpeg", 2, 3), "meal.jpeg", "image/jpeg")
		require.NoError(t, err)
		assert.Equal(t, ".jpg", result.Extension)
		assert.Equal(t, "image/jpeg", result.MediaType)
		config, format, err := image.DecodeConfig(bytes.NewReader(result.Data))
		require.NoError(t, err)
		assert.Equal(t, "jpeg", format)
		assert.Equal(t, 2, config.Width)
		assert.Equal(t, 3, config.Height)
	})

	t.Run("normalizes png", func(t *testing.T) {
		result, err := NormalizeRecipeImage(encodedImage(t, "png", 2, 2), "meal.png", "image/png")
		require.NoError(t, err)
		assert.Equal(t, ".png", result.Extension)
		assert.Equal(t, "image/png", result.MediaType)
	})

	t.Run("rejects mismatched metadata", func(t *testing.T) {
		data := encodedImage(t, "png", 2, 2)
		_, err := NormalizeRecipeImage(data, "meal.jpg", "image/jpeg")
		assert.Error(t, err)
	})

	t.Run("rejects oversized dimensions before decode", func(t *testing.T) {
		data := encodedImage(t, "png", MaxRecipeImageAxis+1, 1)
		_, err := NormalizeRecipeImage(data, "wide.png", "image/png")
		assert.Error(t, err)
	})
}

package icon

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"strings"
)

// MaxBytes 是单个图标文件上限（256 KiB）。
const MaxBytes = 256 << 10

// MaxDimension 是图标单边像素上限。
const MaxDimension = 1024

// MaxPixels 是图标总像素上限，防止解压炸弹。
const MaxPixels = 1 << 20

// ErrInvalid 表示图标内容不合法。
var ErrInvalid = errors.New("图标内容不合法")

var (
	magicPNG  = []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}
	magicGIFa = []byte("GIF87a")
	magicGIFb = []byte("GIF89a")
	magicRIFF = []byte("RIFF")
	magicWEBP = []byte("WEBP")
)

// Validate 校验图标字节：大小、真实格式、尺寸，返回规范 mime 与扩展名。
// 仅接受 PNG / JPEG / GIF / ICO；SVG、HTML、WebP 等一律拒绝。
func Validate(data []byte) (mime, ext string, err error) {
	if len(data) == 0 {
		return "", "", fmt.Errorf("%w：空文件", ErrInvalid)
	}
	if len(data) > MaxBytes {
		return "", "", fmt.Errorf("%w：超过 %d 字节", ErrInvalid, MaxBytes)
	}
	head := data
	if len(head) > 512 {
		head = head[:512]
	}
	lower := strings.ToLower(string(head))
	if strings.Contains(lower, "<svg") || strings.Contains(lower, "<html") || strings.Contains(lower, "<!doctype") {
		return "", "", fmt.Errorf("%w：不允许的文本/矢量内容", ErrInvalid)
	}

	switch {
	case bytes.HasPrefix(data, magicPNG):
		if err := checkDecoded(data); err != nil {
			return "", "", err
		}
		return "image/png", ".png", nil
	case bytes.HasPrefix(data, []byte{0xff, 0xd8, 0xff}):
		if err := checkDecoded(data); err != nil {
			return "", "", err
		}
		return "image/jpeg", ".jpg", nil
	case bytes.HasPrefix(data, magicGIFa) || bytes.HasPrefix(data, magicGIFb):
		if err := checkDecoded(data); err != nil {
			return "", "", err
		}
		return "image/gif", ".gif", nil
	case bytes.HasPrefix(data, magicRIFF) && len(data) >= 16 && bytes.Equal(data[8:12], magicWEBP):
		return "", "", fmt.Errorf("%w：不支持 WebP 图标", ErrInvalid)
	case isICO(data):
		// ICO 尺寸在目录项中，逐项校验。
		if err := checkICO(data); err != nil {
			return "", "", err
		}
		return "image/x-icon", ".ico", nil
	default:
		return "", "", fmt.Errorf("%w：仅支持 PNG/JPEG/GIF/ICO", ErrInvalid)
	}
}

func checkDecoded(data []byte) error {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("%w：图片解码失败", ErrInvalid)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return fmt.Errorf("%w：尺寸非法", ErrInvalid)
	}
	if cfg.Width > MaxDimension || cfg.Height > MaxDimension {
		return fmt.Errorf("%w：尺寸超过 %dx%d", ErrInvalid, MaxDimension, MaxDimension)
	}
	if int64(cfg.Width)*int64(cfg.Height) > MaxPixels {
		return fmt.Errorf("%w：像素数超过 %d", ErrInvalid, MaxPixels)
	}
	return nil
}

func isICO(data []byte) bool {
	if len(data) < 6 {
		return false
	}
	return binary.LittleEndian.Uint16(data[0:2]) == 0 && binary.LittleEndian.Uint16(data[2:4]) == 1
}

func checkICO(data []byte) error {
	count := int(binary.LittleEndian.Uint16(data[4:6]))
	if count == 0 || count > 64 {
		return fmt.Errorf("%w：ICO 图像数量非法", ErrInvalid)
	}
	if len(data) < 6+count*16 {
		return fmt.Errorf("%w：ICO 目录不完整", ErrInvalid)
	}
	for i := 0; i < count; i++ {
		off := 6 + i*16
		w := int(data[off])
		h := int(data[off+1])
		if w == 0 {
			w = 256
		}
		if h == 0 {
			h = 256
		}
		if w > MaxDimension || h > MaxDimension {
			return fmt.Errorf("%w：ICO 尺寸非法", ErrInvalid)
		}
	}
	return nil
}

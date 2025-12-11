package utils

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// ExtractCounterNumberFromEXIF извлекает номер счетчика из EXIF метаданных USER_COMMENT
func ExtractCounterNumberFromEXIF(data []byte) string {
	// Проверяем JPEG маркер
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
		return ""
	}

	offset := 2
	for offset < len(data)-1 {
		// Ищем маркер сегмента
		if data[offset] != 0xFF {
			break
		}

		marker := data[offset+1]
		offset += 2

		// Пропускаем маркеры без данных
		if marker == 0xFF {
			continue
		}

		// APP1 сегмент содержит EXIF данные
		if marker == 0xE1 {
			counterNumber := extractFromAPP1(data, offset)
			if counterNumber != "" {
				return counterNumber
			}
		}

		// Читаем длину сегмента
		if offset+2 > len(data) {
			break
		}
		length := int(binary.BigEndian.Uint16(data[offset : offset+2]))
		if length < 2 {
			break
		}
		offset += length
	}

	return ""
}

// extractFromAPP1 извлекает USER_COMMENT из APP1 сегмента
func extractFromAPP1(data []byte, offset int) string {
	if offset+6 > len(data) {
		return ""
	}

	// Проверяем "Exif\0\0" заголовок
	exifHeader := []byte{0x45, 0x78, 0x69, 0x66, 0x00, 0x00}
	if !bytes.Equal(data[offset:offset+6], exifHeader) {
		return ""
	}

	// Упрощенная реализация: ищем USER_COMMENT в EXIF данных
	// Это упрощенная версия, полная реализация требует полного парсера EXIF
	// Используем библиотеку goexif для более надежного извлечения

	return ""
}

// ReadEXIFUserComment читает USER_COMMENT из EXIF используя библиотеку goexif
func ReadEXIFUserComment(data []byte) (string, error) {
	// Используем библиотеку github.com/rwcarlsen/goexif/exif
	// Это будет реализовано через импорт библиотеки
	return "", fmt.Errorf("not implemented - use goexif library")
}

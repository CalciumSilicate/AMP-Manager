// Package opencc provides a high-performance singleton wrapper around the OpenCC
// library for Simplified Chinese ↔ Traditional Chinese conversion.
package opencc

import (
	"sync"

	"github.com/longbridgeapp/opencc"
	log "github.com/sirupsen/logrus"
)

var (
	s2tConverter *opencc.OpenCC
	t2sConverter *opencc.OpenCC
	s2tOnce      sync.Once
	t2sOnce      sync.Once
)

func getS2T() *opencc.OpenCC {
	s2tOnce.Do(func() {
		var err error
		s2tConverter, err = opencc.New("s2t")
		if err != nil {
			log.WithError(err).Error("Failed to initialize OpenCC s2t converter")
		}
	})
	return s2tConverter
}

func getT2S() *opencc.OpenCC {
	t2sOnce.Do(func() {
		var err error
		t2sConverter, err = opencc.New("t2s")
		if err != nil {
			log.WithError(err).Error("Failed to initialize OpenCC t2s converter")
		}
	})
	return t2sConverter
}

// SimplifiedToTraditional converts Simplified Chinese text to Traditional Chinese.
// Returns the original text unchanged if the converter is unavailable.
func SimplifiedToTraditional(text string) string {
	conv := getS2T()
	if conv == nil {
		return text
	}
	result, err := conv.Convert(text)
	if err != nil {
		log.WithError(err).Warn("OpenCC s2t conversion failed")
		return text
	}
	return result
}

// TraditionalToSimplified converts Traditional Chinese text to Simplified Chinese.
// Returns the original text unchanged if the converter is unavailable.
func TraditionalToSimplified(text string) string {
	conv := getT2S()
	if conv == nil {
		return text
	}
	result, err := conv.Convert(text)
	if err != nil {
		log.WithError(err).Warn("OpenCC t2s conversion failed")
		return text
	}
	return result
}

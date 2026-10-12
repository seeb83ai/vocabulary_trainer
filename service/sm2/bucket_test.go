package sm2

import (
	"reflect"
	"testing"

	"vocabulary_trainer/models"
)

func TestAnswerBuckets(t *testing.T) {
	yes, no := true, false
	cases := []struct {
		name             string
		mode             string
		pinyinSeen       *bool
		audioPlayed      *bool
		voiceUnavailable bool
		want             []string
	}{
		{"transl_to_zh ignores flags", models.ModeTranslToZh, &yes, &yes, false, []string{BucketToZh}},
		{"zh_to_transl without audio", models.ModeZhToTransl, &no, &no, false, []string{BucketZhOnly}},
		{"zh_to_transl with audio", models.ModeZhToTransl, &no, &yes, false, []string{BucketVoice}},
		{"zh_to_transl ignores pinyin", models.ModeZhToTransl, &yes, &no, false, []string{BucketZhOnly}},
		{"pinyin mode, blurred and not tapped", models.ModeZhPinyinToTransl, &no, &no, false, []string{BucketZhOnly}},
		{"pinyin mode, pinyin seen", models.ModeZhPinyinToTransl, &yes, &no, false, []string{BucketPinyin}},
		{"pinyin mode, pinyin seen and audio played", models.ModeZhPinyinToTransl, &yes, &yes, false, []string{BucketPinyin, BucketVoice}},
		{"voice_to_transl", models.ModeVoiceToTransl, &yes, &yes, false, []string{BucketVoiceOnly}},
		{"voice_to_transl with voice unavailable shows text", models.ModeVoiceToTransl, &no, &no, true, []string{BucketZhOnly}},
		{"voice_to_transl with voice unavailable, audio played", models.ModeVoiceToTransl, &yes, &yes, true, []string{BucketVoice}},
		{"zh_to_transl, old client without flags", models.ModeZhToTransl, nil, nil, false, []string{BucketVoice}},
		{"pinyin mode, old client without flags", models.ModeZhPinyinToTransl, nil, nil, false, []string{BucketPinyin, BucketVoice}},
		{"no_sound, old client without flags", models.ModeZhToTranslNoSound, nil, nil, false, []string{BucketZhOnly}},
		{"unknown mode is not recorded", "new_word", &yes, &yes, false, nil},
		{"no_sound ignores impossible flags", models.ModeZhToTranslNoSound, &yes, &yes, false, []string{BucketZhOnly}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := AnswerBuckets(c.mode, c.pinyinSeen, c.audioPlayed, c.voiceUnavailable)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("AnswerBuckets(%q, %v, %v, %v) = %v, want %v",
					c.mode, deref(c.pinyinSeen), deref(c.audioPlayed), c.voiceUnavailable, got, c.want)
			}
		})
	}
}

func deref(b *bool) any {
	if b == nil {
		return nil
	}
	return *b
}

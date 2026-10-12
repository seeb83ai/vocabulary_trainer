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

func TestHelpBuckets(t *testing.T) {
	yes, no := true, false
	cases := []struct {
		name             string
		mode             string
		pinyinSeen       *bool
		playPressed      *bool
		hasPinyin        bool
		blurPinyin       bool
		voiceUnavailable bool
		want             []string
	}{
		{"blurred pinyin tapped", models.ModeZhPinyinToTransl, &yes, nil, true, true, false,
			[]string{BucketPinyinRevealOffered, BucketPinyinRevealed}},
		{"blurred pinyin not tapped", models.ModeZhPinyinToTransl, &no, nil, true, true, false,
			[]string{BucketPinyinRevealOffered}},
		{"pinyin not blurred: no reveal offered", models.ModeZhPinyinToTransl, &yes, nil, true, false, false, nil},
		{"word without pinyin: no reveal offered", models.ModeZhPinyinToTransl, &no, nil, false, true, false, nil},
		{"old client without pinyin flag", models.ModeZhPinyinToTransl, nil, nil, true, true, false, nil},
		{"zh_to_transl, play pressed", models.ModeZhToTransl, nil, &yes, true, true, false,
			[]string{BucketPlayOffered, BucketPlayPressed}},
		{"zh_to_transl, play not pressed", models.ModeZhToTransl, nil, &no, true, false, false,
			[]string{BucketPlayOffered}},
		{"pinyin mode, revealed and play pressed", models.ModeZhPinyinToTransl, &yes, &yes, true, true, false,
			[]string{BucketPinyinRevealOffered, BucketPinyinRevealed, BucketPlayOffered, BucketPlayPressed}},
		{"voice_to_transl: sound is the question", models.ModeVoiceToTransl, nil, &yes, true, false, false, nil},
		{"voice_to_transl with voice unavailable shows play", models.ModeVoiceToTransl, nil, &yes, true, false, true,
			[]string{BucketPlayOffered, BucketPlayPressed}},
		{"no_sound: no play button", models.ModeZhToTranslNoSound, nil, &yes, true, false, false, nil},
		{"transl_to_zh: no help", models.ModeTranslToZh, &yes, &yes, true, true, false, nil},
		{"old client without play flag", models.ModeZhToTransl, nil, nil, true, false, false, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := HelpBuckets(c.mode, c.pinyinSeen, c.playPressed, c.hasPinyin, c.blurPinyin, c.voiceUnavailable)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("HelpBuckets(%q, %v, %v, hasPinyin=%v, blur=%v, voiceUnavailable=%v) = %v, want %v",
					c.mode, deref(c.pinyinSeen), deref(c.playPressed), c.hasPinyin, c.blurPinyin, c.voiceUnavailable, got, c.want)
			}
		})
	}
}

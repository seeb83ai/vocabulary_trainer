package sm2

import "vocabulary_trainer/models"

// Answer buckets group answers by the help the user had on the card.
const (
	BucketZhOnly    = "zh_only"
	BucketPinyin    = "pinyin"
	BucketVoice     = "voice"
	BucketVoiceOnly = "voice_only"
	BucketToZh      = "to_zh"
)

// AnswerBuckets returns the buckets an answer counts in. The client reports
// whether the user saw the pinyin and played the audio before answering;
// flags the mode makes impossible are ignored, and a missing flag (old client)
// counts as used when the card offered that help. An answer with pinyin and
// audio counts in both buckets. Returns nil for modes that are not recorded.
func AnswerBuckets(mode string, pinyinSeen, audioPlayed *bool, voiceUnavailable bool) []string {
	var canSeePinyin, canHearAudio bool
	switch mode {
	case models.ModeTranslToZh:
		return []string{BucketToZh}
	case models.ModeZhToTranslNoSound:
	case models.ModeVoiceToTransl:
		if !voiceUnavailable {
			return []string{BucketVoiceOnly}
		}
		// The client shows the hanzi and a play button instead.
		canHearAudio = true
	case models.ModeZhToTransl:
		canHearAudio = true
	case models.ModeZhPinyinToTransl:
		canSeePinyin, canHearAudio = true, true
	default:
		return nil
	}
	var buckets []string
	if canSeePinyin && (pinyinSeen == nil || *pinyinSeen) {
		buckets = append(buckets, BucketPinyin)
	}
	if canHearAudio && (audioPlayed == nil || *audioPlayed) {
		buckets = append(buckets, BucketVoice)
	}
	if len(buckets) == 0 {
		return []string{BucketZhOnly}
	}
	return buckets
}

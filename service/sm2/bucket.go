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

// Help buckets count the cards where the user could reveal the pinyin or
// press play, and the cards where they did. Attempts and errors keep their
// meaning: answers and wrong answers in that situation.
const (
	BucketPinyinRevealOffered = "pinyin_reveal_offered"
	BucketPinyinRevealed      = "pinyin_revealed"
	BucketPlayOffered         = "play_offered"
	BucketPlayPressed         = "play_pressed"
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

// HelpBuckets returns the help buckets an answer counts in: a pinyin reveal
// is offered when the card's pinyin hint is blurred, play is offered when the
// question screen shows the play button. A missing flag (old client) records
// nothing, so the use rates stay exact.
func HelpBuckets(mode string, pinyinSeen, playPressed *bool, hasPinyin, blurPinyin, voiceUnavailable bool) []string {
	var buckets []string
	if mode == models.ModeZhPinyinToTransl && hasPinyin && blurPinyin && pinyinSeen != nil {
		buckets = append(buckets, BucketPinyinRevealOffered)
		if *pinyinSeen {
			buckets = append(buckets, BucketPinyinRevealed)
		}
	}
	// On a voice-only card the sound is the question, so pressing play is
	// not a help.
	playShown := mode == models.ModeZhToTransl || mode == models.ModeZhPinyinToTransl ||
		(mode == models.ModeVoiceToTransl && voiceUnavailable)
	if playShown && playPressed != nil {
		buckets = append(buckets, BucketPlayOffered)
		if *playPressed {
			buckets = append(buckets, BucketPlayPressed)
		}
	}
	return buckets
}

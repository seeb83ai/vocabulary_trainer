package migrate

import (
	"database/sql"
	"fmt"
	"strings"
)

// v20261018200000 adds the examples from the tables of
// https://studycli.org/learn-chinese/chinese-grammar/ to the shared library
// (user 1) as the importable list "grammar". Sentences also get "grammar_s".
// A word the library already has only gets the tags; its glosses stay as they
// are. A new word gets the page pinyin (without spaces, except in sentences)
// and the EN and DE glosses below. A library refresh later replaces these
// glosses with the dictionary ones where CC-CEDICT/HanDeDict has the word.
// It runs only on a library that already has glosses (see addGrammarExamples).
func init() {
	register(migration{
		version: 20261018200000,
		fn:      addGrammarExamples,
	})
}

type grammarExample struct {
	zh       string
	pinyin   string
	en       []string
	de       []string
	sentence bool
}

// Glosses contain no "," or "/": CheckAnswer accepts each part on its own.
var grammarExamples = []grammarExample{
	// Sentences.
	{"我吃。", "Wǒ chī.", []string{"I eat."}, []string{"Ich esse."}, true},
	{"她去。", "Tā qù.", []string{"She goes."}, []string{"Sie geht."}, true},
	{"我去超市。", "Wǒ qù chāoshì.", []string{"I go to the supermarket."}, []string{"Ich gehe in den Supermarkt."}, true},
	{"她吃面包。", "Tā chī miànbāo.", []string{"She eats bread."}, []string{"Sie isst Brot."}, true},
	{"你喜欢猫。", "Nǐ xǐhuan māo.", []string{"You like cats."}, []string{"Du magst Katzen."}, true},
	{"昨天他去了超市。", "Zuótiān tā qù le chāoshì.", []string{"Yesterday he went to the supermarket."}, []string{"Gestern ist er in den Supermarkt gegangen."}, true},
	{"他昨天去了超市。", "Tā zuótiān qù le chāoshì.", []string{"He went to the supermarket yesterday."}, []string{"Er ist gestern in den Supermarkt gegangen."}, true},
	{"我在桂林工作。", "Wǒ zài Guìlín gōngzuò.", []string{"I work in Guilin."}, []string{"Ich arbeite in Guilin."}, true},
	{"我昨天在家看书。", "Wǒ zuótiān zài jiā kàn shū.", []string{"I read books at home yesterday."}, []string{"Ich habe gestern zu Hause Bücher gelesen."}, true},
	{"我住在桂林。", "Wǒ zhù zài Guìlín.", []string{"I live in Guilin."}, []string{"Ich wohne in Guilin."}, true},
	{"我吃面包。", "Wǒ chī miànbāo.", []string{"I eat bread."}, []string{"Ich esse Brot."}, true},
	{"你吃面包。", "Nǐ chī miànbāo.", []string{"You eat bread."}, []string{"Du isst Brot."}, true},
	{"我们吃面包。", "Wǒmen chī miànbāo.", []string{"We eat bread."}, []string{"Wir essen Brot."}, true},
	{"他们吃面包。", "Tāmen chī miànbāo.", []string{"They eat bread."}, []string{"Sie essen Brot."}, true},
	{"他去超市了。", "Tā qù chāoshì le.", []string{"He went to the supermarket."}, []string{"Er ist in den Supermarkt gegangen."}, true},
	{"我去过。", "Wǒ qù guo.", []string{"I've been there."}, []string{"Ich war schon dort."}, true},
	{"她戴着一条围巾。", "Tā dài zhe yī tiáo wéijīn.", []string{"She's wearing a scarf."}, []string{"Sie trägt einen Schal."}, true},
	{"她在工作。", "Tā zài gōngzuò.", []string{"She's working."}, []string{"Sie arbeitet gerade."}, true},
	{"她正在工作。", "Tā zhèngzài gōngzuò.", []string{"She's working right now."}, []string{"Sie arbeitet genau jetzt."}, true},
	{"我有一个问题。", "Wǒ yǒu yī gè wèntí.", []string{"I have a problem."}, []string{"Ich habe ein Problem."}, true},
	{"我有五个问题。", "Wǒ yǒu wǔ gè wèntí.", []string{"I have five problems."}, []string{"Ich habe fünf Probleme."}, true},
	{"我有九十九个问题。", "Wǒ yǒu jiǔshíjiǔ gè wèntí.", []string{"I have 99 problems.", "I have ninety-nine problems."}, []string{"Ich habe 99 Probleme.", "Ich habe neunundneunzig Probleme."}, true},
	{"你要去超市。", "Nǐ yào qù chāoshì.", []string{"You want to go to the supermarket."}, []string{"Du willst in den Supermarkt gehen."}, true},
	{"你要去超市吗？", "Nǐ yào qù chāoshì ma?", []string{"Do you want to go to the supermarket?"}, []string{"Willst du in den Supermarkt gehen?"}, true},
	{"他喜欢猫。", "Tā xǐhuan māo.", []string{"He likes cats."}, []string{"Er mag Katzen."}, true},
	{"他喜欢猫吗？", "Tā xǐhuan māo ma?", []string{"Does he like cats?"}, []string{"Mag er Katzen?"}, true},
	{"这是我的猫。", "Zhè shì wǒ de māo.", []string{"This is my cat."}, []string{"Das ist meine Katze."}, true},
	{"你的围巾很漂亮。", "Nǐ de wéijīn hěn piàoliang.", []string{"Your scarf is beautiful."}, []string{"Dein Schal ist schön."}, true},
	{"这条围巾不漂亮。", "Zhè tiáo wéijīn bù piàoliang.", []string{"This scarf is not beautiful."}, []string{"Dieser Schal ist nicht schön."}, true},
	{"我不要去超市。", "Wǒ bù yào qù chāoshì.", []string{"I do not want to go to the supermarket."}, []string{"Ich will nicht in den Supermarkt gehen."}, true},
	{"他不吃面包。", "Tā bù chī miànbāo.", []string{"He does not eat bread."}, []string{"Er isst kein Brot."}, true},
	{"我没有猫。", "Wǒ méiyǒu māo.", []string{"I do not have any cats."}, []string{"Ich habe keine Katzen."}, true},
	{"他们没有面包。", "Tāmen méiyǒu miànbāo.", []string{"They do not have any bread."}, []string{"Sie haben kein Brot."}, true},
	{"我有一只猫。", "Wǒ yǒu yī zhī māo.", []string{"I have a cat."}, []string{"Ich habe eine Katze."}, true},
	{"他喜欢这本书。", "Tā xǐhuan zhè běn shū.", []string{"He likes this book."}, []string{"Er mag dieses Buch."}, true},

	// Phrases.
	{"三位老师", "sān wèi lǎoshī", []string{"3 teachers", "three teachers"}, []string{"3 Lehrer", "drei Lehrer"}, false},
	{"三十个学生", "sānshí gè xuésheng", []string{"30 students", "thirty students"}, []string{"30 Schüler", "dreißig Schüler"}, false},
	{"八个孩子", "bā gè háizi", []string{"8 children", "eight children"}, []string{"8 Kinder", "acht Kinder"}, false},

	// Time words.
	{"早上", "zǎoshang", []string{"morning"}, []string{"Morgen"}, false},
	{"中午", "zhōngwǔ", []string{"noon"}, []string{"Mittag"}, false},
	{"下午", "xiàwǔ", []string{"afternoon"}, []string{"Nachmittag"}, false},
	{"晚上", "wǎnshang", []string{"evening"}, []string{"Abend"}, false},
	{"今天", "jīntiān", []string{"today"}, []string{"heute"}, false},
	{"明天", "míngtiān", []string{"tomorrow"}, []string{"morgen"}, false},
	{"前天", "qiántiān", []string{"day before yesterday"}, []string{"vorgestern"}, false},
	{"后天", "hòutiān", []string{"day after tomorrow"}, []string{"übermorgen"}, false},
	{"上个星期", "shàng gè xīngqī", []string{"last week"}, []string{"letzte Woche"}, false},
	{"下个星期", "xià gè xīngqī", []string{"next week"}, []string{"nächste Woche"}, false},
	{"上个月", "shàng gè yuè", []string{"last month"}, []string{"letzten Monat"}, false},
	{"下个月", "xià gè yuè", []string{"next month"}, []string{"nächsten Monat"}, false},
	{"去年", "qùnián", []string{"last year"}, []string{"letztes Jahr"}, false},
	{"明年", "míngnián", []string{"next year"}, []string{"nächstes Jahr"}, false},

	// Pronouns and plurals with 们.
	{"我们", "wǒmen", []string{"we"}, []string{"wir"}, false},
	{"他们", "tāmen", []string{"they (all male or mixed gender group)"}, []string{"sie (männlich oder gemischt)"}, false},
	{"她们", "tāmen", []string{"they (female group)"}, []string{"sie (weiblich)"}, false},
	{"你们", "nǐmen", []string{"you (plural)"}, []string{"ihr"}, false},
	{"学生", "xuésheng", []string{"student"}, []string{"Schüler", "Student"}, false},
	{"学生们", "xuéshengmen", []string{"students"}, []string{"Schüler (Plural)", "Studenten"}, false},
	{"老师", "lǎoshī", []string{"teacher"}, []string{"Lehrer"}, false},
	{"老师们", "lǎoshīmen", []string{"teachers"}, []string{"Lehrer (Plural)"}, false},
	{"孩子", "háizi", []string{"child"}, []string{"Kind"}, false},
	{"孩子们", "háizimen", []string{"children"}, []string{"Kinder"}, false},
	{"女士", "nǚshì", []string{"lady"}, []string{"Dame"}, false},
	{"女士们", "nǚshìmen", []string{"ladies"}, []string{"Damen"}, false},
	{"先生", "xiānsheng", []string{"gentleman"}, []string{"Herr"}, false},
	{"先生们", "xiānshengmen", []string{"gentlemen"}, []string{"Herren"}, false},
	{"朋友", "péngyou", []string{"friend"}, []string{"Freund"}, false},
	{"朋友们", "péngyoumen", []string{"friends"}, []string{"Freunde"}, false},

	// Question words.
	{"谁", "shéi", []string{"who"}, []string{"wer"}, false},
	{"什么", "shénme", []string{"what"}, []string{"was"}, false},
	{"哪里", "nǎlǐ", []string{"where"}, []string{"wo"}, false},
	{"哪儿", "nǎr", []string{"where"}, []string{"wo"}, false},
	{"为什么", "wèi shénme", []string{"why"}, []string{"warum"}, false},
	{"哪个", "nǎge", []string{"which"}, []string{"welcher", "welche", "welches"}, false},
	{"什么时候", "shénme shíhou", []string{"when"}, []string{"wann"}, false},
	{"怎么", "zěnme", []string{"how"}, []string{"wie"}, false},

	// Possessives.
	{"我的", "wǒ de", []string{"mine", "my"}, []string{"mein", "meins"}, false},
	{"你的", "nǐ de", []string{"yours", "your"}, []string{"dein", "deins"}, false},
	{"他的", "tā de", []string{"his"}, []string{"sein", "seins"}, false},
	{"她的", "tā de", []string{"hers", "her"}, []string{"ihr", "ihrs"}, false},
	{"我们的", "wǒmen de", []string{"ours", "our"}, []string{"unser", "unseres"}, false},
	{"你们的", "nǐmen de", []string{"yours (plural)", "your (plural)"}, []string{"euer", "eures"}, false},
	{"他们的", "tāmen de", []string{"theirs", "their"}, []string{"ihr (Plural)", "ihres (Plural)"}, false},

	// Measure words.
	{"个", "gè", []string{"the most common measure word"}, []string{"allgemeines Zählwort"}, false},
	{"只", "zhī", []string{"measure word for animals"}, []string{"Zählwort für Tiere"}, false},
	{"本", "běn", []string{"measure word for books"}, []string{"Zählwort für Bücher"}, false},
	{"辆", "liàng", []string{"measure word for vehicles"}, []string{"Zählwort für Fahrzeuge"}, false},
	{"块", "kuài", []string{"measure word for pieces of objects and for money"}, []string{"Zählwort für Stücke und für Geld"}, false},
	{"封", "fēng", []string{"measure word for letters"}, []string{"Zählwort für Briefe"}, false},
	{"张", "zhāng", []string{"measure word for flat objects (like paper)"}, []string{"Zählwort für flache Dinge (z. B. Papier)"}, false},
	{"瓶", "píng", []string{"measure word for bottles"}, []string{"Zählwort für Flaschen"}, false},
	{"杯", "bēi", []string{"measure word for cups"}, []string{"Zählwort für Tassen"}, false},
	{"双", "shuāng", []string{"measure word for pairs (of things)"}, []string{"Zählwort für Paare"}, false},

	// Grammar terms.
	{"语法", "yǔfǎ", []string{"grammar"}, []string{"Grammatik"}, false},
	{"句子", "jùzi", []string{"sentence"}, []string{"Satz"}, false},
	{"词", "cí", []string{"word"}, []string{"Wort"}, false},
	{"主语", "zhǔyǔ", []string{"subject"}, []string{"Subjekt"}, false},
	{"动词", "dòngcí", []string{"verb"}, []string{"Verb"}, false},
	{"宾语", "bīnyǔ", []string{"object"}, []string{"Objekt"}, false},
	{"名词", "míngcí", []string{"noun"}, []string{"Nomen", "Substantiv"}, false},
	{"形容词", "xíngróngcí", []string{"adjective"}, []string{"Adjektiv"}, false},
	{"副词", "fùcí", []string{"adverb"}, []string{"Adverb"}, false},
	{"代词", "dàicí", []string{"pronoun"}, []string{"Pronomen"}, false},
	{"量词", "liàngcí", []string{"measure word", "classifier"}, []string{"Zählwort", "Klassifikator"}, false},
	{"助词", "zhùcí", []string{"particle"}, []string{"Partikel"}, false},
	{"时间词", "shíjiāncí", []string{"time word"}, []string{"Zeitausdruck"}, false},
	{"语序", "yǔxù", []string{"word order"}, []string{"Wortstellung"}, false},
	{"声调", "shēngdiào", []string{"tone"}, []string{"Ton"}, false},
	{"拼音", "pīnyīn", []string{"pinyin"}, []string{"Pinyin"}, false},
	{"汉字", "hànzì", []string{"Chinese character"}, []string{"chinesisches Schriftzeichen"}, false},
	{"否定", "fǒudìng", []string{"negation"}, []string{"Verneinung"}, false},
	{"疑问句", "yíwènjù", []string{"interrogative sentence", "question"}, []string{"Fragesatz"}, false},
	{"单数", "dānshù", []string{"singular"}, []string{"Singular", "Einzahl"}, false},
	{"复数", "fùshù", []string{"plural"}, []string{"Plural", "Mehrzahl"}, false},
	{"同音词", "tóngyīncí", []string{"homophone"}, []string{"Homophon"}, false},
	{"例句", "lìjù", []string{"example sentence"}, []string{"Beispielsatz"}, false},
	{"练习", "liànxí", []string{"to practice", "exercise"}, []string{"üben", "Übung"}, false},
}

func addGrammarExamples(db *sql.DB) error {
	// Only a filled library gets the list. Test and fresh databases have no
	// library glosses; there the new words would show up in every test, and
	// on a database that still waits for the library prefill at startup
	// (LibraryNeedsPrefill) these glosses would stop that prefill.
	var filled bool
	if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM translations t JOIN words z ON z.id = t.zh_word_id
		WHERE z.user_id = 1 AND z.language = 'zh')`).Scan(&filled); err != nil {
		return fmt.Errorf("check library glosses: %w", err)
	}
	if !filled {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	grammarTag, err := grammarTagID(tx, "grammar", "Grammar examples (studycli.org)")
	if err != nil {
		return err
	}
	sentenceTag, err := grammarTagID(tx, "grammar_s", "Grammar example sentences (studycli.org)")
	if err != nil {
		return err
	}

	for _, e := range grammarExamples {
		var zhID int64
		err := tx.QueryRow(`SELECT id FROM words WHERE text = ? AND language = 'zh' AND user_id = 1`, e.zh).Scan(&zhID)
		if err == sql.ErrNoRows {
			pinyin := e.pinyin
			if !e.sentence {
				// Library pinyin is written without spaces between syllables.
				pinyin = strings.ReplaceAll(pinyin, " ", "")
			}
			res, err := tx.Exec(`INSERT INTO words (text, language, pinyin, user_id) VALUES (?, 'zh', ?, 1)`, e.zh, pinyin)
			if err != nil {
				return fmt.Errorf("insert %s: %w", e.zh, err)
			}
			if zhID, err = res.LastInsertId(); err != nil {
				return err
			}
			for lang, glosses := range map[string][]string{"en": e.en, "de": e.de} {
				for _, g := range glosses {
					if err := linkGrammarGloss(tx, zhID, g, lang); err != nil {
						return fmt.Errorf("gloss %q of %s: %w", g, e.zh, err)
					}
				}
			}
		} else if err != nil {
			return fmt.Errorf("find %s: %w", e.zh, err)
		}

		tags := []int64{grammarTag}
		if e.sentence {
			tags = append(tags, sentenceTag)
		}
		for _, tagID := range tags {
			if _, err := tx.Exec(`INSERT OR IGNORE INTO word_tags (word_id, tag_id) VALUES (?, ?)`, zhID, tagID); err != nil {
				return fmt.Errorf("tag %s: %w", e.zh, err)
			}
		}
	}
	return tx.Commit()
}

// grammarTagID returns the global tag name, created if needed, marked
// importable. The description is set only where the tag has none.
func grammarTagID(tx *sql.Tx, name, description string) (int64, error) {
	var id int64
	err := tx.QueryRow(`SELECT id FROM tags WHERE name = ? ORDER BY id LIMIT 1`, name).Scan(&id)
	if err == sql.ErrNoRows {
		res, err := tx.Exec(`INSERT INTO tags (name, description, importable) VALUES (?, ?, 1)`, name, description)
		if err != nil {
			return 0, fmt.Errorf("insert tag %s: %w", name, err)
		}
		return res.LastInsertId()
	}
	if err != nil {
		return 0, fmt.Errorf("find tag %s: %w", name, err)
	}
	if _, err := tx.Exec(`UPDATE tags SET importable = 1,
		description = CASE WHEN description = '' THEN ? ELSE description END
		WHERE id = ?`, description, id); err != nil {
		return 0, fmt.Errorf("update tag %s: %w", name, err)
	}
	return id, nil
}

// linkGrammarGloss links the library gloss (text, lang) to the zh word, as a
// gloss without frequency rank.
func linkGrammarGloss(tx *sql.Tx, zhID int64, text, lang string) error {
	if _, err := tx.Exec(`INSERT OR IGNORE INTO words (text, language, user_id) VALUES (?, ?, 1)`, text, lang); err != nil {
		return err
	}
	var glossID int64
	if err := tx.QueryRow(`SELECT id FROM words WHERE text = ? AND language = ? AND user_id = 1`, text, lang).Scan(&glossID); err != nil {
		return err
	}
	_, err := tx.Exec(`INSERT OR IGNORE INTO translations (translation_word_id, zh_word_id, source, rank) VALUES (?, ?, 'user', NULL)`, glossID, zhID)
	return err
}

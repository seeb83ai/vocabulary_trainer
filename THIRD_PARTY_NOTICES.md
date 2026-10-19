# Third-party notices

The code of this project has the [MIT license](LICENSE). This project also uses data, fonts and code from other projects. This file lists them with their licenses. The app shows the same list at `/credits`.

The full license texts of the files that are in this repository are in [`service/frontend/licenses/`](service/frontend/licenses/). The binary includes them, and the app serves them at `/licenses/<file>`.

## Files in this repository

| Files | Source | License | Changes |
|---|---|---|---|
| `service/frontend/chart.js` | [Chart.js](https://www.chartjs.org/) 4.5.1, © 2014-2024 Chart.js Contributors | MIT ([text](service/frontend/licenses/chartjs-mit.txt)) | None |
| `service/frontend/html2canvas.min.js` | [html2canvas](https://html2canvas.hertzen.com/) 1.4.1, © 2012 Niklas von Hertzen | MIT ([text](service/frontend/licenses/html2canvas-mit.txt)) | None |
| `service/frontend/tailwind.js` | [Tailwind CSS](https://tailwindcss.com/) 3.4.17 (Play CDN build), © Tailwind Labs, Inc. The build includes other MIT packages, which keep their notices in the file. | MIT ([text](service/frontend/licenses/tailwindcss-mit.txt)) | None |
| `service/frontend/fonts/plus-jakarta-sans-latin.woff2` | [Plus Jakarta Sans](https://github.com/tokotype/PlusJakartaSans), © 2020 The Plus Jakarta Sans Project Authors | SIL OFL 1.1 ([text](service/frontend/licenses/plus-jakarta-sans-ofl.txt)) | Latin subset, WOFF2 |
| `service/frontend/fonts/noto-sans-sc-subset.woff2` | [Noto Sans SC](https://github.com/notofonts/noto-cjk), © 2014-2021 Adobe, with Reserved Font Name "Source" | SIL OFL 1.1 ([text](service/frontend/licenses/noto-sans-sc-ofl.txt)) | Subset of the characters, WOFF2 |
| `service/cmd/import-frequency/frequency_data*.txt`, `service/db/migrate/frequency_data*.txt` | [FrequencyWords](https://github.com/hermitdave/FrequencyWords) by Hermit Dave, `content/2018/{zh_cn,en,de}/*_50k.txt`, made from [OpenSubtitles](https://www.opensubtitles.org/) | CC BY-SA 4.0 ([text](service/frontend/licenses/cc-by-sa-4.0.txt)) | Filtered to entries of only CJK ideographs (zh) or only letters (en, de), duplicates removed, cut to the first 8,000 entries. Our changed lists have the same license, CC BY-SA 4.0. |
| `service/db/migrate/v20261018200000_add_grammar_examples.go` | Example sentences from the [studycli.org Chinese grammar guide](https://studycli.org/learn-chinese/chinese-grammar/) | © studycli.org, no license given. See [#608](https://github.com/seeb83ai/vocabulary_trainer/issues/608). | German translations added |

## Data that the import tools download

The repository does not include these files. The tools download them, and the app shows the imported data.

| Data | Tool | License |
|---|---|---|
| [CC-CEDICT](https://www.mdbg.net/chinese/dictionary?page=cedict) | `import-cedict -lang en` | [CC BY-SA 4.0](https://creativecommons.org/licenses/by-sa/4.0/) |
| [HanDeDict](https://handedict.zydeo.net/) | `import-cedict -lang de` | [CC BY-SA 2.0 DE](https://creativecommons.org/licenses/by-sa/2.0/de/) |
| [Make Me a Hanzi](https://github.com/skishore/makemeahanzi) `dictionary.txt` | `import-hanzi` | [LGPL-3.0 or later](https://github.com/skishore/makemeahanzi/blob/master/COPYING) |
| [complete-hsk-vocabulary](https://github.com/drkameleon/complete-hsk-vocabulary) | `import-hsk` | [MIT](https://github.com/drkameleon/complete-hsk-vocabulary/blob/main/LICENSE) |
| [mp3-chinese-pinyin-sound](https://github.com/davinfifield/mp3-chinese-pinyin-sound) | `pinyin_download.sh`, `import-pinyin` | [Unlicense](https://unlicense.org/) (public domain) |

## Go modules

The Go modules in `service/go.mod` have permissive licenses (MIT, BSD-2-Clause, BSD-3-Clause). The repository does not include their source code. If you give a built binary or image to other people, include their license texts, for example with [go-licenses](https://github.com/google/go-licenses).

## npm packages

`package.json` has only development dependencies (tests). The app does not include them.

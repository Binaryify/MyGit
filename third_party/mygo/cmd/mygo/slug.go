package main

import (
	_ "embed"
	"strings"
	"sync"
)

//go:generate go run gen_pinyin.go

// slugify makes s a slug, lower-case letters and digits with single
// hyphens between words, or "app" without any. Chinese characters take
// their most common Mandarin reading, each its own word, as yun-yin-le of
// 云音乐 (Japanese kanji too, which have no reading of their own here), and
// kana and Hangul their romanization, as ramen of ラーメン and seoul of 서울.
func slugify(s string) string {
	var b []byte
	gap := false // a hyphen goes before the next word
	word := func(w string) {
		if gap && len(b) > 0 {
			b = append(b, '-')
		}
		gap = false
		b = append(b, w...)
	}
	rs := []rune(strings.ToLower(s))
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case 'a' <= r && r <= 'z' || '0' <= r && r <= '9':
			word(string(r))
		case mandarin(r) != "":
			gap = true
			word(mandarin(r))
			gap = true
		case isHangul(r):
			var next rune
			if i+1 < len(rs) {
				next = rs[i+1]
			}
			word(hangul(r, next))
		case kanaIndex(r) >= 0:
			w, n := kana(rs[i:])
			word(w)
			i += n - 1
		default:
			gap = true
		}
	}
	if len(b) == 0 {
		return "app"
	}
	return string(b)
}

// pinyinData holds the readings of CJK Unified Ideographs, as
// gen_pinyin.go writes them.
//
//go:embed pinyin.bin
var pinyinData string

var pinyin = sync.OnceValues(func() ([]string, string) {
	syllables, table, _ := strings.Cut(pinyinData, "\n")
	return strings.Fields(syllables), table
})

// mandarin returns the reading of a Chinese character without its tone,
// or "".
func mandarin(r rune) string {
	if r < 0x4E00 || r > 0x9FFF {
		return ""
	}
	syllables, table := pinyin()
	i := int(r-0x4E00) * 2
	if n := int(table[i])<<8 | int(table[i+1]); n > 0 {
		return syllables[n-1]
	}
	return ""
}

// The Revised Romanization of a Hangul syllable's jamo: the initial
// consonants, the vowels, and the final consonants, which sound otherwise
// before a vowel (the next syllable's silent ㅇ), as g of 한국 in 한국어,
// hangugeo.
var (
	hangulInitials     = strings.Split("g,kk,n,d,tt,r,m,b,pp,s,ss,,j,jj,ch,k,t,p,h", ",")
	hangulVowels       = strings.Split("a,ae,ya,yae,eo,e,yeo,ye,o,wa,wae,oe,yo,u,wo,we,wi,yu,eu,ui,i", ",")
	hangulFinals       = strings.Split(",k,k,k,n,n,n,t,l,k,m,l,l,l,p,l,m,p,p,t,t,ng,t,t,k,t,p,t", ",")
	hangulFinalsLinked = strings.Split(",g,kk,ks,n,nj,n,d,r,lg,lm,lb,ls,lt,lp,r,m,b,ps,s,ss,ng,j,ch,k,t,p,", ",")
)

func isHangul(r rune) bool { return 0xAC00 <= r && r <= 0xD7A3 }

// hangul romanizes the Hangul syllable r, which next follows.
func hangul(r, next rune) string {
	s := int(r - 0xAC00)
	finals := hangulFinals
	if isHangul(next) && int(next-0xAC00)/588 == 11 {
		finals = hangulFinalsLinked
	}
	return hangulInitials[s/588] + hangulVowels[s%588/28] + finals[s%28]
}

// kanaRomaji is the Hepburn romanization of hiragana from U+3041 (ぁ) to
// U+3096 (ゖ), whose katakana come 0x60 later; kana handles the small ones
// and っ.
var kanaRomaji = strings.Fields(`a a i i u u e e o o ka ga ki gi ku gu ke ge ko go
	sa za shi ji su zu se ze so zo ta da chi ji tsu tsu zu te de to do
	na ni nu ne no ha ba pa hi bi pi fu bu pu he be pe ho bo po
	ma mi mu me mo ya ya yu yu yo yo ra ri ru re ro wa wa i e o n vu ka ke`)

// kanaIndex returns the position of a kana in kanaRomaji, or -1.
func kanaIndex(r rune) int {
	switch {
	case 0x3041 <= r && r <= 0x3096:
		return int(r - 0x3041)
	case 0x30A1 <= r && r <= 0x30F6:
		return int(r - 0x30A1)
	}
	return -1
}

// kana romanizes the kana that rs starts with as one word, and returns how
// many runes it took.
func kana(rs []rune) (string, int) {
	var b []byte
	double := false // after っ
	n := 0
	for ; n < len(rs); n++ {
		r := rs[n]
		if r == 'ー' {
			continue // a long vowel, which the word leaves out, as ramen
		}
		i := kanaIndex(r)
		if i < 0 {
			break
		}
		if r == 'っ' || r == 'ッ' {
			double = true
			continue
		}
		romaji := kanaRomaji[i]
		last := len(b) - 1
		switch {
		case (i == 0x42 || i == 0x44 || i == 0x46) && last > 0 && b[last] == 'i':
			// ゃ, ゅ and ょ after the i of a syllable: kya of きゃ, sha of しゃ.
			b = b[:last]
			if strings.HasSuffix(string(b), "sh") || strings.HasSuffix(string(b), "ch") || strings.HasSuffix(string(b), "j") {
				romaji = romaji[1:]
			}
		case i <= 8 && i%2 == 0 && last >= 0 && strings.IndexByte("aiueo", b[last]) >= 0:
			// A small vowel in place of the one before: fa of ファ, ti of ティ,
			// wi of ウィ.
			b = b[:last]
			if last == 0 || strings.IndexByte("aiueo", b[last-1]) >= 0 {
				b = append(b, 'w')
			}
		}
		if double && strings.IndexByte("aiueon", romaji[0]) < 0 {
			if strings.HasPrefix(romaji, "ch") {
				b = append(b, 't')
			} else {
				b = append(b, romaji[0])
			}
		}
		double = false
		b = append(b, romaji...)
	}
	return string(b), n
}

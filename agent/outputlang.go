package agent

// OutputLanguage returns the configured display language. Korean is the default;
// explicit user language choices still apply to reports and conversation output.
var OutputLanguage func() string

const defaultOutputLang = "ko"

var SupportedOutputLangs = []string{"en", "ko", "zh", "es", "ja"}

func normalizeLang(code string) string {
	for _, c := range SupportedOutputLangs {
		if c == code {
			return code
		}
	}
	return defaultOutputLang
}

func resolveOutputLang() string {
	if OutputLanguage != nil {
		if code := OutputLanguage(); code != "" {
			return normalizeLang(code)
		}
	}
	return defaultOutputLang
}

var langPhrase = map[string]string{
	"ko": "한국어", "en": "영어(English)", "zh": "중국어 간체(简体中文)",
	"es": "스페인어(Español)", "ja": "일본어(日本語)",
}

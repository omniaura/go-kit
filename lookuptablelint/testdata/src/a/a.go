package a

const opus = "opus"

var googleAIOpenAICompatAudioFormats = map[string]bool{ // want `static lookup table googleAIOpenAICompatAudioFormats has 2 entries; prefer predicate isGoogleAIOpenAICompatAudioFormats with a single-case switch`
	"mp3": true,
	"wav": true,
}

var supportedImageFormats = map[string]struct{}{ // want `static lookup table supportedImageFormats has 3 entries; prefer predicate isSupportedImageFormats with a single-case switch`
	"gif":  {},
	"jpeg": {},
	"png":  {},
}

var constKeyFormats = map[string]bool{ // want `static lookup table constKeyFormats has 2 entries; prefer predicate isConstKeyFormats with a single-case switch`
	opus:  true,
	"wav": true,
}

var hasFalseValue = map[string]bool{
	"mp3": true,
	"wav": false,
}

var mutableFormats = map[string]bool{
	"mp3": true,
	"wav": true,
}

var rangedFormats = map[string]struct{}{
	"gif": {},
	"png": {},
}

var lenFormats = map[string]bool{
	"gif": true,
	"png": true,
}

var structValueUse = map[string]struct{}{
	"gif": {},
	"png": {},
}

var intLookup = map[int]bool{ // want `static lookup table intLookup has 2 entries; prefer predicate isIntLookup with a single-case switch`
	1: true,
	2: true,
}

var ExportedLookup = map[string]bool{
	"gif": true,
	"png": true,
}

var tooSmall = map[string]bool{
	"gif": true,
}

var tooLarge = map[string]bool{
	"000": true,
	"001": true,
	"002": true,
	"003": true,
	"004": true,
	"005": true,
	"006": true,
	"007": true,
	"008": true,
	"009": true,
	"010": true,
	"011": true,
	"012": true,
	"013": true,
	"014": true,
	"015": true,
	"016": true,
	"017": true,
	"018": true,
	"019": true,
	"020": true,
	"021": true,
	"022": true,
	"023": true,
	"024": true,
	"025": true,
	"026": true,
	"027": true,
	"028": true,
	"029": true,
	"030": true,
	"031": true,
	"032": true,
	"033": true,
	"034": true,
	"035": true,
	"036": true,
	"037": true,
	"038": true,
	"039": true,
	"040": true,
	"041": true,
	"042": true,
	"043": true,
	"044": true,
	"045": true,
	"046": true,
	"047": true,
	"048": true,
	"049": true,
	"050": true,
	"051": true,
	"052": true,
	"053": true,
	"054": true,
	"055": true,
	"056": true,
	"057": true,
	"058": true,
	"059": true,
	"060": true,
	"061": true,
	"062": true,
	"063": true,
	"064": true,
}

func boolLookup(format string) bool {
	return googleAIOpenAICompatAudioFormats[format]
}

func structLookup(format string) bool {
	_, ok := supportedImageFormats[format]
	return ok
}

func constKeyLookup(format string) bool {
	return constKeyFormats[format]
}

func mutate(format string) {
	mutableFormats[format] = true
}

func rangeLookup() int {
	count := 0
	for range rangedFormats {
		count++
	}
	return count
}

func lenLookup() int {
	return len(lenFormats)
}

func structValue(format string) struct{} {
	return structValueUse[format]
}

func intLookupUse(v int) bool {
	return intLookup[v]
}

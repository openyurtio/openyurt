package strings

// IsInStringLst checks if 'str' is in the 'strLst'
func IsInStringLst(strLst []string, str string) bool {
	if len(strLst) == 0 {
		return false
	}
	for _, s := range strLst {
		if str == s {
			return true
		}
	}
	return false
}

package views

// storeRel is the rel of a link to a store. An affiliate link is marked
// "sponsored", as Google asks for paid and affiliate links; every store link
// stays "nofollow", as before.
func storeRel(affiliate bool) string {
	if affiliate {
		return "sponsored nofollow"
	}
	return "nofollow"
}

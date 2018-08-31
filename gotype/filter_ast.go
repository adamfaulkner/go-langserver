package gotype

type identFilter struct {
	all         bool
	identifiers map[string]struct{}
}

func (i *identFilter) checkIdent(ident string) bool {
	if i.all {
		return true
	}
	_, ok := i.identifiers[ident]
	return ok
}

package dataset

// ContentID lets black-box tests give a hand-edited manifest a matching id.
func ContentID(m Manifest) string { return m.contentID() }

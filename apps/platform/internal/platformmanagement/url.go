package platformmanagement

import "net/url"

func urlValues(raw string) (url.Values, error) { return url.ParseQuery(raw) }

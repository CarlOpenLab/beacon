package main

func or(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

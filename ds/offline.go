package ds

// SetOffline destroys dropbear's ability to make network requests.
// This helps us ensure no network calls are made during backtesting.
func SetOffline() {
	BulkHttpClient = nil
	FastHTTPClient = nil
	fastWSDialer = nil
}

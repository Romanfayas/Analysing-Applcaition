package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
)

func main() {
	req, _ := http.NewRequest("GET", "https://www.nseindia.com/api/corporates-corporateActions?index=equities&symbol=RELIANCE", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Referer", "https://www.nseindia.com/")
	
	client := &http.Client{}
	
	// Need to hit base page first to get cookies
	reqBase, _ := http.NewRequest("GET", "https://www.nseindia.com", nil)
	reqBase.Header = req.Header
	respBase, _ := client.Do(reqBase)
	
	// Add cookies to api request
	for _, cookie := range respBase.Cookies() {
		req.AddCookie(cookie)
	}
	
	resp, _ := client.Do(req)
	body, _ := ioutil.ReadAll(resp.Body)
	fmt.Printf("%.200s\n", string(body))
}

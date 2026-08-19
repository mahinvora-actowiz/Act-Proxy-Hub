package main

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// type E struct {
// 	Key   string
// 	Value interface{}
// }

type ProxyConfig struct {
	ID                    primitive.ObjectID `bson:"_id,omitempty"`
	APIKey                string             `bson:"apiKey"`
	ProxyName             string             `bson:"proxyName"`
	Token                 string             `bson:"token"`
	TokenID               primitive.ObjectID `bson:"tokenId"`
	TotalEstimatedCredits int                `bson:"totalEstimatedCredits"`
	UsedCredits           int                `bson:"usedCredits"`
	Status                string             `bson:"status"`
}

type RequestLog struct {
	Key          string             `bson:"key"`
	TokenID      primitive.ObjectID `bson:"tokenId"`
	Domain       string             `bson:"domain"`
	IP           string             `bson:"ip"`
	StatusCode   int                `bson:"statusCode"`
	ResponseTime int64              `bson:"responseTime"`
	CreditUsed   int                `bson:"creditUsed"`
	CreatedAt    time.Time          `bson:"createdAt"`
}

// Scrape.do configuration struct
type ScrapeDoConfig struct {
	// Core
	Token string `json:"token"`
	URL   string `json:"url"`

	// Proxy behavior
	Super           *bool  `json:"super"`
	Render          *bool  `json:"render"`
	GeoCode         string `json:"geoCode"`
	RegionalGeoCode string `json:"regionalGeoCode"`
	SessionId       *int   `json:"sessionId"`

	// Headers & cookies
	CustomHeaders  *bool    `json:"customHeaders"`
	ExtraHeaders   *bool    `json:"extraHeaders"`
	ForwardHeaders *bool    `json:"forwardHeaders"`
	SetCookies     []string `json:"setCookies"`

	// Request control
	Timeout            int   `json:"timeout"`
	RetryTimeout       int   `json:"retryTimeout"`
	DisableRetry       *bool `json:"disableRetry"`
	DisableRedirection *bool `json:"disableRedirection"`

	// Rendering
	WaitUntil    string `json:"waitUntil"`
	CustomWait   int    `json:"customWait"`
	WaitSelector string `json:"waitSelector"`

	// Device
	Device string `json:"device"`
	Width  int    `json:"width"`
	Height int    `json:"height"`

	// Output
	ScreenShot           *bool  `json:"screenShot"`
	FullScreenShot       *bool  `json:"fullScreenShot"`
	ParticularScreenShot string `json:"particularScreenShot"`
	Output               string `json:"output"`
	ReturnJSON           *bool  `json:"returnJSON"`

	// Advanced
	BlockResources        *bool `json:"blockResources"`
	PlayWithBrowser       *bool `json:"playWithBrowser"`
	TransparentResponse   *bool `json:"transparentResponse"`
	ShowFrames            *bool `json:"showFrames"`
	ShowWebsocketRequests *bool `json:"showWebsocketRequests"`
	PureCookies           *bool `json:"pureCookies"`

	// Callback
	Callback string `json:"callback"`
}

// ScraperAPI configuration struct
type ScraperAPIConfig struct {
	// Core
	Token string `json:"token"`
	URL   string `json:"url"`

	// Proxy behavior - ScraperAPI specific
	Premium         *bool  `json:"premium"`
	UltraPremium    *bool  `json:"ultra_premium"`
	Render          *bool  `json:"render"`
	CountryCode     string `json:"country_code"`
	RegionalGeoCode string `json:"regionalGeoCode"`
	Zip             string `json:"zip"`
	SessionNumber   *int   `json:"sessionNumber"`

	// Headers & cookies
	KeepHeaders *bool    `json:"keep_headers"`
	SetCookies  []string `json:"setCookies"`

	// Request control
	Timeout        int   `json:"timeout"`
	MaxRetries     int   `json:"max_retry_attempts"`
	FollowRedirect *bool `json:"follow_redirect"`

	// Rendering
	WaitForSelector string `json:"wait_for_selector"`
	CustomWait      int    `json:"customWait"`

	// Device
	DeviceType string `json:"device_type"`

	// Output
	Screenshot   *bool  `json:"screenshot"`
	OutputFormat string `json:"output_format"`
	AutoParse    *bool  `json:"autoParse"`

	// Advanced
	BlockResources *bool `json:"blockResources"`
}

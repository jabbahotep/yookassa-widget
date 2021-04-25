package yookassa_widget

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/Diggernaut/cast"
	"github.com/Diggernaut/viper"
	"github.com/gorilla/mux"
	"github.com/natefinch/lumberjack"
)

var (
	cfg            *viper.Viper
	shopID         string
	secretKey      string
	idempotenceKey string
	locale         string
)

type Amount struct {
	Value    float64            `json:"amount"`
	Currency string				`json:"currency"`
}

type Confirmation struct {
	Type              string	`json:"type"`
	Locale            string    `json:"locale"`
	ConfirmationToken string    `json:"confirmation_token"`
}

type Recipient struct {
	AccountID    string         `json:"account_id"`
	GatewayID    string			`json:"gateway_id"`
}

type PaymentRequest struct {
	Amount       Amount         `json:"amount"`
	Confirmation Confirmation	`json:"confirmation"`
	Capture      bool			`json:"capture"`
	Description  string			`json:"description"`
}

type PaymentResponse struct {
	ID           string         `json:"id"`
	Status       string         `json:"status"`
	Paid         bool			`json:"paid"`
	Amount       Amount         `json:"amount"`
	Confirmation Confirmation	`json:"confirmation"`
	CreatedAt    string			`json:"created_at"`
	Description  string			`json:"description"`
	Metadata     interface{}	`json:"metadata"`
	Recipient    Recipient      `json:"recipient"`
	Refundable   bool			`json:"refundable"`
	Test         bool			`json:"test"`
}


func init() {
	// SET UP LOGGER
	log.SetOutput(&lumberjack.Logger{
		Filename:   "/var/log/yookassa_widget.log",
		MaxSize:    100, // megabytes
		MaxBackups: 3,   // max files
		MaxAge:     7,   // days
	})
	//log.SetOutput(os.Stdout)

	// READING CONFIG
	cfg = viper.New()
	cfg.SetConfigName("config")
	cfg.AddConfigPath("./")
	err := cfg.ReadInConfig()
	if err != nil {
		log.Fatalf("Error: cannot read config. Reason: %v\n", err)
	}
	shopID = cfg.GetString("shopID")
	secretKey = cfg.GetString("secretKey")
	idempotenceKey = cfg.GetString("idempotenceKey")
	locale = cfg.GetString("locale")
}

func main() {
	log.Println("Yoomoney Service started")
	router := mux.NewRouter()
	router.HandleFunc(`/create_payment`, createPayment).Methods("POST")
	sslCert := cfg.GetString("sslCert")
	privateKey := cfg.GetString("privateKey")
	if sslCert != "" && privateKey != "" {
		err := http.ListenAndServeTLS(cfg.GetString("bindIP")+":"+cfg.GetString("bindPort"), sslCert, privateKey, router)
		if err != nil {
			log.Fatalln(err)
		}
	} else {
		err := http.ListenAndServe(cfg.GetString("bindIP")+":"+cfg.GetString("bindPort"), router)
		if err != nil {
			log.Fatalln(err)
		}
	}
}

func createPayment(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	defer finishHandle(&w)
	// SETTING UP RESPONSE HEADERS: CORS AND CONTENT-TYPE
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	var body = new(struct {
		invoice   int                   `json:"invoice"`
		amount    float64               `json:"amount"`
		currency  string                `json:"currency"`
	})

	err := json.NewDecoder(r.Body).Decode(body)
	if err != nil {
		errorResponse(400, err.Error(), &w)
		return
	}

	if body.invoice <= 0 {
		errorResponse(400, "invalid or missing invoice number", &w)
		return
	}

	if body.amount <= 0 {
		errorResponse(400, "invalid or missing amount", &w)
		return
	}

	if body.currency != "USD" && body.currency != "GBP" && body.currency != "RUB" {
		errorResponse(400, "invalid or missing currency, supported USD, GBP, RUB", &w)
		return
	}

	url := "https://api.yookassa.ru/v3/payments"
	description := "Invoice " + strconv.Itoa(body.invoice)

	query := PaymentRequest {
		Amount: Amount {
			Value: body.amount,
			Currency: body.currency,
		},
		Confirmation: Confirmation{
			Type: "embedded",
			Locale: locale,
		},
		Capture: true,
		Description: description,
	}

	jsonQuery, err := json.Marshal(query)
	if err != nil {
		errorResponse(400, err.Error(), &w)
		return
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonQuery))
	req.Header.Set(shopID, secretKey)
	req.Header.Set("Idempotence-Key", idempotenceKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		errorResponse(400, err.Error(), &w)
		return
	}
	defer resp.Body.Close()

	var presponse PaymentResponse
	err = json.NewDecoder(resp.Body).Decode(presponse)
	if err != nil {
		errorResponse(400, err.Error(), &w)
		return
	}

	response := make(map[string]interface{})
	response["status"] = "success"
	response["result"] = presponse
	bytedata, _ := json.Marshal(response)
	w.WriteHeader(http.StatusOK)
	w.Write(bytedata)
}

func errorResponse(code int, error string, w *http.ResponseWriter) {
	response := make(map[string]string)
	response["status"] = "failure"
	response["error"] = error
	bytedata, _ := json.Marshal(response)
	http.Error(*w, string(bytedata), code)
}

func finishHandle(w *http.ResponseWriter) {
	if x := recover(); x != nil {
		log.Printf("Run time panic: %v\n", x)
		errorResponse(500, cast.ToString(x), w)
	}
}

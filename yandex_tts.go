/*
Library for  use  Yandex cloud TTS
Can auto update  Auth Keys(IAmToken), save audio to wave AND alaw from lpcm without external sox utility (Purpose: make 
integrity for  Asterisk,mono, rate 8000 Hz)
Can save to ogg file optionaly
TODO:
Make optional save IAMToken in Redis
Make more  parameters in func for generate audio data ( ADD rate,  speed, emotion, voice )
*/
package yandex_tts

import (
	"bytes"
	"crypto/rsa"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/go-audio/audio"
	"github.com/go-audio/wav"
	"github.com/pkg/errors"
	"github.com/zaf/g711"

	"github.com/dgrijalva/jwt-go"
)

const (
	apiUrl       = "https://tts.api.cloud.yandex.net"
	resource     = "/speech/v1/tts:synthesize"
	templateTime = "2006-01-02T15:04:05.000000Z" //template  for parsing timestamp
)

type TTSYandex struct {
	IamToken              string
	KeyID                 string
	ServiceAccountID      string
	KeyFile               string
	ExpiredTokenTime      time.Time
	guardUpdateToken      sync.RWMutex
	//guardExpiredTokenTime sync.RWMutex
	//guardUpdateProcess    sync.Mutex //
	UpdateInProcess       bool
	errorUpdateCount      int
	request               Request
	PrivateKey            *rsa.PrivateKey
	//UpdateTokenChannel    chan struct{} //канал  для немедленного обновления токена
}

//iamtoken and exp time  from  request
type TokenWithExpTime struct {
	IAMToken  string `json:"iamToken"`
	ExpiresAt string `json:"expiresAt"`
}

//make  response  to generate audio stream
//format lpcm or oggopus
func (y *TTSYandex) MakeAudioWav(message string) ([]byte, error) {
	buffer := new(bytes.Buffer)
	//t.request.Text = message
	currentRequest := Request(y.request) //здесь  копия   ?
	currentRequest.Text = message
	data := structToMap(&currentRequest)

	log.Println(data.Encode())
	buffer.WriteString(data.Encode())

	u, _ := url.ParseRequestURI(apiUrl)
	u.Path = resource
	urlStr := u.String() //
	//fmt.Println("URL string: ", urlStr)

	client := &http.Client{Timeout: time.Duration(2 * time.Second)}
	r, _ := http.NewRequest("POST", urlStr, buffer) // URL-encoded payload
	//fmt.Println(r.URL)

	r.Header.Add("Authorization", "Bearer "+y.IamToken)

	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(r)
	if err != nil {
		return nil, err //
	}
	defer resp.Body.Close() //здесь аккуратнее что бы не получить nil в resp и панику в runtime
	if resp.StatusCode != http.StatusOK {
		log.Println("Код ошибки :", resp.StatusCode)
		message_error := "Error  while request MakeAudioWav"
		return nil, errors.Wrap(errors.New(message_error), strconv.Itoa(resp.StatusCode))
	}
	log.Println(resp.Status)
	data_file, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return data_file, nil // здесь в err по идее будет nil

}

//
func (y *TTSYandex) MakeAudioOgg(message string) ([]byte, error) {
	buffer := new(bytes.Buffer)
	currentRequest := Request(y.request) //здесь  копия
	currentRequest.Text = message
	currentRequest.Format = "oggopus"

	data := structToMap(&currentRequest)
	delete(data, "sampleRateHertz") //можно и без этого

	log.Println(data.Encode())
	buffer.WriteString(data.Encode())

	u, _ := url.ParseRequestURI(apiUrl)
	u.Path = resource
	urlStr := u.String() //
	//log.Println("URL string: ", urlStr)

	client := &http.Client{Timeout: time.Duration(2 * time.Second)}
	r, _ := http.NewRequest("POST", urlStr, buffer) // URL-encoded payload
	//fmt.Println(r.URL)

	r.Header.Add("Authorization", "Bearer "+y.IamToken)

	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(r)
	if err != nil {
		//log.Println("Ошибка из DO", err)
		return nil, err //
	}
	defer resp.Body.Close() //здесь аккуратнее что бы не получить nil в resp и панику в runtime
	if resp.StatusCode != http.StatusOK {
		log.Println("Код ошибки :", resp.StatusCode)

		message_error := "Error  while request MakeAudioOgg"
		return nil, errors.Wrap(errors.New(message_error), strconv.Itoa(resp.StatusCode))

	}
	log.Println(resp.Status)
	data_file, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return data_file, nil // здесь в err по идее будет nil

}

//
func (y *TTSYandex) SignedToken() (string, error) {
	issuedAt := time.Now()
	token := jwt.NewWithClaims(ps256WithSaltLengthEqualsHash, jwt.StandardClaims{
		Issuer:    y.ServiceAccountID,
		IssuedAt:  issuedAt.Unix(),
		ExpiresAt: issuedAt.Add(time.Hour).Unix(),
		Audience:  "https://iam.api.cloud.yandex.net/iam/v1/tokens",
	})
	token.Header["kid"] = y.KeyID

	//privateKey := loadPrivateKey()
	signed, err := token.SignedString(y.PrivateKey)
	if err != nil {
		return "", err
	}
	return signed, err
}

//
func (y *TTSYandex) GetIAMToken() (TokenWithExpTime, error) {
	jot, err := y.SignedToken()
	if err != nil {
		log.Println("Ошибка при подписи")
		return *new(TokenWithExpTime), err
	}
	resp, err := http.Post(
		"https://iam.api.cloud.yandex.net/iam/v1/tokens",
		"application/json",
		strings.NewReader(fmt.Sprintf(`{"jwt":"%s"}`, jot)),
	)
	if err != nil {
		log.Println("Ошибка при POST получении токена")
		return *new(TokenWithExpTime), err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		//body, _ := ioutil.ReadAll(resp.Body)
		//fmt.Sprintf("%s: %s", resp.Status, body)
		log.Println("Status code: ", resp.StatusCode)
		log.Println("Вернули не 200 при получении токена !")
		message_error := "Can not get IAMToken"
		return *new(TokenWithExpTime), errors.Wrap(errors.New(message_error), strconv.Itoa(resp.StatusCode))

	}
	tokenStructInst := new(TokenWithExpTime)

	err = json.NewDecoder(resp.Body).Decode(&tokenStructInst)
	if err != nil {
		log.Println("Ошибка при декодировании tokenstruct")
		return *new(TokenWithExpTime), err
	}
	return *tokenStructInst, err
}

//
func (y *TTSYandex) SetIAMToken(token TokenWithExpTime) error {
	y.guardUpdateToken.Lock()
	y.IamToken = token.IAMToken
	timeExp, err := time.Parse(templateTime, token.ExpiresAt)

	if err != nil {
		log.Println("Ошибка при парсинге EXP даты ")
		y.guardUpdateToken.Unlock()
		return errors.Wrap(err, "Error while parsing Exp time")
	}
	y.ExpiredTokenTime = timeExp
	y.guardUpdateToken.Unlock()
	return nil
}

type Request struct {
	Format          string
	SampleRateHertz int
	Text            string
	Voice           string
	Emotion         string
	FolderId        string
	Lang            string
}

//Фабричный   метод  для  создания  объекта  TTSYandex
func NewYandexTTS(keyFile string, keyID string, serviceAccountID string, folderId string) (*TTSYandex, error) {
	// проверяем key file
	data, err := ioutil.ReadFile(keyFile)
	if err != nil {
		return &TTSYandex{}, errors.Wrap(err, "Can not read  private key")
	}
	rsaPrivateKey, err := jwt.ParseRSAPrivateKeyFromPEM(data)
	if err != nil {
		return &TTSYandex{}, errors.Wrap(err, "Can not parse PEM")
	}
	//
	instYandexTTS := &TTSYandex{ //
		IamToken:         "",
		KeyID:            keyID,
		ServiceAccountID: serviceAccountID,
		KeyFile:          keyFile,
		PrivateKey:       rsaPrivateKey,

		request: Request{
			Format:          "lpcm",
			SampleRateHertz: 8000,
			Text:            "",
			FolderId:        folderId,
			Emotion:         "good",
			Lang:            "ru-RU",
			Voice:           "oksana"}}
	tokenData, err := instYandexTTS.GetIAMToken()
	//err = err.(*StatusError)
	log.Println("Error: ", err)
	//log.Printf("%+v", tokenData) //для отладки потом убрать
	//log.Println(tokenData.IAMToken)
	if err != nil {

		log.Println("Error  while get token")
		log.Println("Cause code: ", errors.Cause(err))
		return &TTSYandex{}, err
	}
	//ExpiresAt
	instYandexTTS.IamToken = tokenData.IAMToken

	timeExp, err := time.Parse(templateTime, tokenData.ExpiresAt)

	if err != nil {
		//panic(fmt.Sprintf("%v", err)) //для отладки  потом  изменить
		log.Println("Ошибка при парсинге даты  ")
		return &TTSYandex{}, errors.Wrap(err, "Error while parsing Exp time")
	}

	instYandexTTS.ExpiredTokenTime = timeExp
	//log.Println("Exp time in time format : ", instYandexTTS.ExpiredTokenTime) //для отладки

	//Run  updater for token auto update
	//TODO:  make (for  select case ) for different channels
	go func(ya *TTSYandex) {
		ticker := time.NewTicker(15 * time.Minute) //проверяем каждые 15 мин
		for t := range ticker.C {
			nowTime := time.Now()
			elapse := ya.ExpiredTokenTime.Sub(nowTime).Minutes()
			log.Println("Elapse: ", elapse)
			if elapse >= 60 { //за 1 час до протухания токена пробуем обновиться
				continue
			}
			log.Println("Tick at", t)
			log.Println("Old  exp time is equal: ", ya.ExpiredTokenTime)
			tokenWithExpTime, err := ya.GetIAMToken()
			if err != nil {
				log.Println("Ошибка при получении токена")
				log.Fatal("Cause code: ", errors.Cause(err)) //For  testing  only. Replace in prod
			}
			err = ya.SetIAMToken(tokenWithExpTime)
			if err != nil {

				log.Println("Ошибка при установки  токена")
				log.Fatal("Cause code: ", errors.Cause(err)) //not for production!
			}
			//log.Println("New  exp time is equal: ", ya.ExpiredTokenTime)
			//log.Println("New  IAMTOKEN time is equal: ", ya.IamToken)

		}
	}(instYandexTTS)
	return instYandexTTS, err
}

// По умолчанию Go RSA PSS использует PSSSaltLengthAuto,
// но на странице https://tools.ietf.org/html/rfc7518#section-3.5 сказано, что
// размер значения соли должен совпадать с размером вывода хеш-функции.
// После исправления https://github.com/dgrijalva/jwt-go/issues/285
// можно будет заменить на jwt.SigningMethodPS256
var ps256WithSaltLengthEqualsHash = &jwt.SigningMethodRSAPSS{
	SigningMethodRSA: jwt.SigningMethodPS256.SigningMethodRSA,
	Options: &rsa.PSSOptions{
		SaltLength: rsa.PSSSaltLengthEqualsHash,
	},
}

//function for convert struct to  map  url.Values for make HTTP request
func structToMap(i interface{}) (values url.Values) {
	values = url.Values{}
	iVal := reflect.ValueOf(i).Elem()
	typ := iVal.Type()
	for i := 0; i < iVal.NumField(); i++ {
		f := iVal.Field(i)
		var v string
		switch f.Interface().(type) {
		case int, int8, int16, int32, int64:
			v = strconv.FormatInt(f.Int(), 10)
		case uint, uint8, uint16, uint32, uint64:
			v = strconv.FormatUint(f.Uint(), 10)
		case float32:
			v = strconv.FormatFloat(f.Float(), 'f', 4, 32)
		case float64:
			v = strconv.FormatFloat(f.Float(), 'f', 4, 64)
		case []byte:
			v = string(f.Bytes())
		case string:
			v = f.String()
		}

		values.Set(LcFirst(typ.Field(i).Name), v)
	}
	return
}

// first letter to lower case (using  in structToMap for make api request right)
func LcFirst(str string) string {
	for i, v := range str {
		return string(unicode.ToLower(v)) + str[i+1:]
	}
	return ""
}

//Func for Save  LPCM (only!) audio data to alaw  format without  external util sox !!
func SaveLpcmToAlaw(filename string, audioData []byte) error {

	alawFile, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755) //здесь будет wav file
	if err != nil {
		log.Println(err)
		return err
	}
	encoder, err := g711.NewAlawEncoder(alawFile, g711.Lpcm)
	if err != nil {
		log.Printf("Failed to create Writer: %s\n", err)
		return err

	}
	_, err = encoder.Write(audioData)
	if err != nil {
		log.Printf("Encoding failed: %s\n", err)
		return err
	}
	//	}
	return nil
}

//func for make buffer , to save LPCM audio data to WAV file
func NewAudioIntBuffer(r io.Reader) (*audio.IntBuffer, error) {
	buf := audio.IntBuffer{
		Format: &audio.Format{
			NumChannels: 1,
			SampleRate:  8000,
		},
	}
	for {
		var sample int16
		err := binary.Read(r, binary.LittleEndian, &sample)
		switch {
		case err == io.EOF:
			return &buf, nil
		case err != nil:
			return nil, err
		}
		buf.Data = append(buf.Data, int(sample))
	}
}

//Save LPCM  (only!) audio data  to wav file
func SaveAsWave(filename string, audioData []byte) error {
	waveFile, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755) //здесь будет wav file
	if err != nil {
		log.Println(err)
		return err
	}
	waveEncoder := wav.NewEncoder(waveFile, 8000, 16, 1, 1)
	// Create new audio.IntBuffer.
	audioBuf, err := NewAudioIntBuffer(bytes.NewReader(audioData))
	if err != nil {
		log.Println(err)
		return err
	}
	// Write buffer to output file. This writes a RIFF header and the PCM chunks from the audio.IntBuffer.
	if err := waveEncoder.Write(audioBuf); err != nil {
		log.Println(err)
		return err

	}
	if err := waveEncoder.Close(); err != nil {
		log.Println(err)
		return err

	}

	return nil
}

//Save OGG  (only!) audio data  to ogg file
func SaveAsOgg(filename string, audioData []byte) error {
	// oggFile, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755) //здесь будет wav file
	// if err != nil {
	// 	log.Println(err)
	// 	return err
	// }
	if err := ioutil.WriteFile(filename, audioData, 0644); err != nil {
		return errors.New("Error  make output  file")
	}

	return nil
}

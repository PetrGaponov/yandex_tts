# yandex_tts
Библиотека для использования  Yandex cloud TTS. Требует  сервисного аккаунта в Yandex Cloud. Использует  авторизацию с помощью IAM-токена.  При конструировании объекта YandexTTS фабрикой NewYandexTTS запускается отдельная go рутина которая отслеживает время жизни токена и обновляет его до истечения срока. (токен живет 12 часов)  
Библиотеку можно использовать, например,  в своих REST - RPC сервисах для генерации голоса из текста  
Удобна для интеграции с телефонией Asterisk.
Инструкции по получению сервисного аккаунта и регистрации здесь:https://cloud.yandex.ru/docs/iam/operations/iam-token/create-for-sa  
Пример использования библиотеки :
```go
package main  

import (  
	"fmt"  
	"log"  
	//"pr_yandex_tts"  
	pr_yandex_tts "github.com/PetrGaponov/yandex_tts"  

	"github.com/pkg/errors"  
)  

const (
	keyID            = "xxxxxxxxxxxxxxxxxxxx"
	serviceAccountID = "xxxxxxxxxxxxxxxxxxxx"
	keyFile          = "private.pem"
	FolderId         = "xxxxxxxxxxxxxxxxxxxx"
	)

func main() {
	var exitCh = make(chan struct{})
	//func New(keyFile string, keyID string, serviceAccountID string) (*TTSYandex, error) {
	newTTS, err := pr_yandex_tts.NewYandexTTS(keyFile, keyID, serviceAccountID, FolderId)
	if err != nil {
		//panic(fmt.Sprintf("%+v", err))
		fmt.Println(err.Error())
		log.Fatal("Cause code: ", errors.Cause(err))
		//log.Fatal(err.Status())

	}
	fmt.Printf("%+v", newTTS)
	audioFileWav, err := newTTS.MakeAudioWav("еще одна новая проверка связи 2")
	//err = err.(StatusError)
	if err != nil {
		//panic(fmt.Sprintf("%+v", err))
		fmt.Println(err.Error())
		log.Fatal(errors.Cause(err))

	}
	err = pr_yandex_tts.SaveAsWave("new_wav2.wav", audioFileWav)
	if err != nil {
		log.Println(err)
	}

	audioFileOgg, err = newTTS.MakeAudioOgg("еще одна новая проверка связи с OGG")
	//err = err.(StatusError)
	if err != nil {
		//panic(fmt.Sprintf("%+v", err))
		fmt.Println(err.Error())
		log.Fatal(errors.Cause(err))

	}
	err = pr_yandex_tts.SaveAsOgg("new_ogg.ogg", audioFileOgg)
	if err != nil {
		//panic(fmt.Sprintf("%+v", err))
		fmt.Println(err.Error())
		log.Fatal(errors.Cause(err))

	}
	//
	err = pr_yandex_tts.SaveLpcmToAlaw("new_ogg.alaw", audioFileWave)
	if err != nil {
		//panic(fmt.Sprintf("%+v", err))
		fmt.Println(err.Error())
		log.Fatal(errors.Cause(err))

	}
	//

	//log.Println("Waiting ....")
	// <-exitCh

}
```

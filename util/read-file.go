package util

import (
	"io/ioutil"
	"log"
)

func GetMinerAddr() string {
	content, err := ioutil.ReadFile("minerAddr")
	if err != nil {
		log.Fatal("Cannot read file: minerAddr")
	}
	return string(content)
}

func GetMinerPrivateKey() string {
	content, err := ioutil.ReadFile("minerPrivKey")
	if err != nil {
		log.Fatal("Cannot read file: minerPrivKey")
	}
	return string(content)
}
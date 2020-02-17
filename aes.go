package libraries

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"strings"
)

//加密字符串
func AES_cfb_encrypt(strMesg string, strKey string, ivkey string) (result string, err error) {
	myhash := Newhash()
	key := []byte(myhash.Hash(strKey, "", "32")) //由hash补齐32位key
	if strings.Count(ivkey, "") < 16 {
		ivkey = myhash.Hash(ivkey)
	}
	var iv = []byte(ivkey)[:aes.BlockSize]
	encrypted := make([]byte, len(strMesg))
	aesBlockEncrypter, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	aesEncrypter := cipher.NewCFBEncrypter(aesBlockEncrypter, iv)
	aesEncrypter.XORKeyStream(encrypted, []byte(strMesg))
	result = base64.StdEncoding.EncodeToString(encrypted)
	return
}

//解密字符串
func AES_cfb_decrypt(srcstr string, strKey string, ivkey string) (strDesc string, err error) {
	myhash := Newhash()
	key := []byte(myhash.Hash(strKey, "", "32")) //由hash补齐32位key
	if strings.Count(ivkey, "") < 16 {
		ivkey = myhash.Hash(ivkey)
	}
	var iv = []byte(ivkey)[:aes.BlockSize]
	src, err := base64.StdEncoding.DecodeString(srcstr)
	if err != nil {
		return "", err
	}
	decrypted := make([]byte, len(src))
	var aesBlockDecrypter cipher.Block
	aesBlockDecrypter, err = aes.NewCipher([]byte(key))
	if err != nil {
		return "", err
	}
	aesDecrypter := cipher.NewCFBDecrypter(aesBlockDecrypter, iv)
	aesDecrypter.XORKeyStream(decrypted, src)
	return string(decrypted), nil
}

//加密字符串
func AES_cfb_Urlencrypt(strMesg string, strKey string, ivkey string) (result string, err error) {
	myhash := Newhash()
	key := []byte(myhash.Hash(strKey, "", "32")) //由hash补齐32位key
	if strings.Count(ivkey, "") < 16 {
		ivkey = myhash.Hash(ivkey)
	}
	var iv = []byte(ivkey)[:aes.BlockSize]
	encrypted := make([]byte, len(strMesg))
	aesBlockEncrypter, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	aesEncrypter := cipher.NewCFBEncrypter(aesBlockEncrypter, iv)
	aesEncrypter.XORKeyStream(encrypted, []byte(strMesg))
	result = base64.URLEncoding.EncodeToString(encrypted)
	return
}

//解密字符串
func AES_cfb_Urldecrypt(srcstr string, strKey string, ivkey string) (strDesc string, err error) {
	myhash := Newhash()
	key := []byte(myhash.Hash(strKey, "", "32")) //由hash补齐32位key
	if strings.Count(ivkey, "") < 16 {
		ivkey = myhash.Hash(ivkey)
	}
	var iv = []byte(ivkey)[:aes.BlockSize]
	src, err := base64.URLEncoding.DecodeString(srcstr)
	if err != nil {
		return "", err
	}
	decrypted := make([]byte, len(src))
	var aesBlockDecrypter cipher.Block
	aesBlockDecrypter, err = aes.NewCipher([]byte(key))
	if err != nil {
		return "", err
	}
	aesDecrypter := cipher.NewCFBDecrypter(aesBlockDecrypter, iv)
	aesDecrypter.XORKeyStream(decrypted, src)
	return string(decrypted), nil
}

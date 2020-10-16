package libraries

import (
	//"github.com/astaxie/beego"
	//"github.com/sillydong/fastimage"
	"fmt"
	"reflect"
	"strings"
	//"strconv"
	//"regexp"
	"math"
	//"os"
	"bytes"
	"unsafe"
)

type Myhash struct {
	Hmac string
	buf  bytes.Buffer
	buf2 bytes.Buffer
	buf3 bytes.Buffer
}

func Newhash() *Myhash {
	h := new(Myhash)
	h.buf.Grow(64)
	h.buf3.Grow(64)
	h.buf2.Grow(1024 * 1024)
	return h
}

const CODE62 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
const CODE_LENTH = 62

var EDOC = map[string]int{"0": 0, "1": 1, "2": 2, "3": 3, "4": 4, "5": 5, "6": 6, "7": 7, "8": 8, "9": 9, "A": 10, "B": 11, "C": 12, "D": 13, "E": 14, "F": 15, "G": 16, "H": 17, "I": 18, "J": 19, "K": 20, "L": 21, "M": 22, "N": 23, "O": 24, "P": 25, "Q": 26, "R": 27, "S": 28, "T": 29, "U": 30, "V": 31, "W": 32, "X": 33, "Y": 34, "Z": 35, "a": 36, "b": 37, "c": 38, "d": 39, "e": 40, "f": 41, "g": 42, "h": 43, "i": 44, "j": 45, "k": 46, "l": 47, "m": 48, "n": 49, "o": 50, "p": 51, "q": 52, "r": 53, "s": 54, "t": 55, "u": 56, "v": 57, "w": 58, "x": 59, "y": 60, "z": 61}

/**
 * 编码 整数 为 base62 字符串
 */
func Base62_Encode(number int) string {
	if number == 0 {
		return "0"
	}
	result := make([]byte, 0)
	for number > 0 {
		round := number / CODE_LENTH
		remain := number % CODE_LENTH
		result = append(result, CODE62[remain])
		number = round
	}
	return string(result)
}

/**
 * 解码字符串为整数
 */
func Base62_Decode(str string) int {
	str = strings.TrimSpace(str)
	var result int = 0
	for index, char := range []byte(str) {
		result += EDOC[string(char)] * int(math.Pow(CODE_LENTH, float64(index)))
	}
	return result
}

func (h *Myhash) Hash(text ...string) string {
	bb := h.S2B(&text[0])
	b := make([]byte, len(bb))
	copy(b, bb)
	if len(b) < 17 {
		hex := []byte{84, 132, 208, 48, 203, 33, 214, 6, 173, 140, 172, 227, 23, 205, 112, 177, 173}
		b = append(b, hex...)
	}
	hex := []byte{146, 124, 150, 213, 72, 186, 154, 4, 126}
	var hmac string
	if len(text) == 1 {
		hmac = ""
	} else {
		hmac = text[1]
	}
	out_type := "16"
	if len(text) == 3 && text[2] == "32" {
		out_type = "32"
	}
	h.buf.Reset()
	h.buf.Write(h.S2B(&hmac))
	if h.buf.Len() < 9 {
		h.buf.Write(hex)
	}

	for true {
		h._h()
		if h.buf.Len() < 9 {
			break
		}
	}
	hm := make([]byte, h.buf.Len())
	copy(hm, h.buf.Bytes())
	for true {
		b = h._hash(b, hm)
		if len(b) < 17 {
			break
		}
	}
	h.buf.Reset()
	if out_type == "32" {
		for _, value := range b {
			h.buf.WriteString(fmt.Sprintf("%02x", value))
		}
		return h.buf.String()
	} else {
		for _, value := range b {
			h.buf.WriteByte(CODE62[value%CODE_LENTH])
		}
		return h.buf.String()
	}
	return ""
}
func (h *Myhash) _hash(b []byte, h1 []byte) []byte {
	h.buf2.Reset()
	s := 40
	for i := 0; i < len(b)/s+1; i++ {
		h.buf.Reset()
		if i*s+s > len(b) {
			h.buf.Write(b[i*s : len(b)])
		} else {
			h.buf.Write(b[i*s : i*s+s])
		}
		h.buf.Write(h1)
		h.__hash()
		h.buf2.Write(h.buf.Bytes())
	}

	return h.buf2.Bytes()
}
func (h *Myhash) __hash() {
	for true {
		h._h()
		if h.buf.Len() < 17 {
			break
		}
	}
}

func (h *Myhash) _h() {
	hex := h.buf.Bytes()[h.buf.Bytes()[0]%uint8(h.buf.Len())]
	//hex := (*b)[int((*b)[0])%len(*b)]
	//tmp := make([]byte, len(*b)-1)
	h.buf3.Reset()
	for i := 0; i < h.buf.Len()-1; i++ {
		h.buf3.WriteByte(h.buf.Bytes()[i] + h.buf.Bytes()[i+1] + hex)
	}
	h.buf.Reset()
	h.buf.Write(h.buf3.Bytes())
}
func (h *Myhash) S2B(s *string) []byte {
	return *(*[]byte)(unsafe.Pointer((*reflect.SliceHeader)(unsafe.Pointer(s))))
}

func (h *Myhash) B2S(buf []byte) string {
	return *(*string)(unsafe.Pointer(&buf))
}

func init() {
	//fmt.Println(Newhash().Hash("123", "123", "32"))
}

package libraries

import (
	"bytes"
	"math/rand"
	"unsafe"

	//"github.com/sillydong/fastimage"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"net"
	"net/url"
	"os"
	"reflect"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"net/http"
	_ "net/http/pprof"

	"github.com/dlclark/regexp2"
	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/pgzip"
	//"path/filepath"
	//
)

var http_c chan *http.Client
var MsgBuf_chan = make(chan *MsgBuffer, runtime.NumCPU())

func init() {
	http_c = make(chan *http.Client, runtime.NumCPU()*5)
	for i := 0; i < runtime.NumCPU(); i++ {
		http_c <- &http.Client{
			Transport: &http.Transport{
				Dial: func(netw, addr string) (net.Conn, error) {
					c, err := net.DialTimeout(netw, addr, time.Second*3) //设置建立连接超时
					if err != nil {
						return nil, err
					}
					c.SetDeadline(time.Now().Add(10 * time.Second)) //设置发送接收数据超时
					return c, nil
				},
			},
		}
		MsgBuf_chan <- &MsgBuffer{}
		writer := new(Pgzip_writer)
		writer.B = new(bytes.Buffer)
		writer.Writer, _ = pgzip.NewWriterLevel(writer.B, 4) //4级解压速度快
		writer.Writer.SetConcurrency(100000, 10)
		pgzipcompress_chan <- writer

		gzip_writer := new(Gzip_writer)
		gzip_writer.Buf = new(MsgBuffer)
		gzip_writer.Writer, _ = gzip.NewWriterLevel(gzip_writer.Buf, 6)
		gzipcompress_chan <- gzip_writer

	}

}

type Pgzip_writer struct {
	B      *bytes.Buffer
	Writer *pgzip.Writer
}
type Buffer_reader struct {
	b *bytes.Buffer
}
type Gzip_writer struct {
	Buf    *MsgBuffer
	Writer *gzip.Writer
}

var pgzipcompress_chan = make(chan *Pgzip_writer, runtime.NumCPU())
var uncompress_chan = make(chan *bytes.Buffer, runtime.NumCPU())
var gzipcompress_chan = make(chan *Gzip_writer, runtime.NumCPU())

func DopgzipCompress(src []byte) []byte {
	g := <-pgzipcompress_chan
	defer func() {
		g.B.Reset()
		pgzipcompress_chan <- g
	}()
	g.Writer.Reset(g.B)
	leng, err := g.Writer.Write(src)
	if err != nil || leng == 0 {
		return nil
	}
	err = g.Writer.Flush()
	if err != nil {
		return nil
	}
	err = g.Writer.Close()
	if err != nil {
		return nil
	}
	b := make([]byte, len(g.B.Bytes()))
	copy(b, g.B.Bytes())
	return b
}

func DopgzipUnCompress(compressSrc []byte) []byte {
	b := <-uncompress_chan
	b.Reset()
	defer func() {
		uncompress_chan <- b
	}()
	b.Write(compressSrc)
	r, err := pgzip.NewReader(b)
	if err != nil {
		return nil
	}
	defer r.Close()
	ndatas, err := ioutil.ReadAll(r)
	res := make([]byte, len(ndatas))
	copy(res, ndatas)
	if err != nil {
		return res
	}
	return res
}

func DogzipCompress(src []byte) []byte {
	g := <-gzipcompress_chan
	defer func() {
		g.Buf.Reset()
		gzipcompress_chan <- g
	}()
	g.Writer.Reset(g.Buf)
	leng, err := g.Writer.Write(src)
	if err != nil || leng == 0 {
		return nil
	}
	err = g.Writer.Flush()
	if err != nil {
		return nil
	}
	err = g.Writer.Close()
	if err != nil {
		return nil
	}
	b := make([]byte, len(g.Buf.Bytes()))
	copy(b, g.Buf.Bytes())
	return b
}

//进行gzip解压缩
func DogzipUnCompress(compressSrc []byte) []byte {
	b := <-uncompress_chan
	defer func() {
		uncompress_chan <- b
	}()
	b.Reset()
	b.Write(compressSrc)
	r, err := gzip.NewReader(b)
	if err != nil {
		return nil
	}
	defer r.Close()
	ndatas, err := ioutil.ReadAll(r)
	res := make([]byte, len(ndatas))
	copy(res, ndatas)
	if err != nil {
		return res
	}
	return res
}
func Void(params ...interface{}) {}

//返回无序map 原array_under_reset变形函数之一
func Map_under_reset(array interface{}, key string, typ int) map[string]interface{} {
	tmp := make(map[string]interface{})
	switch array.(type) {
	case []map[string]string:

		for _, v := range array.([]map[string]string) {
			if typ == 1 {
				tmp[v[key]] = v
			} else if typ == 2 {
				//tmp[v[key]][] = v
			}
		}
	}
	return tmp

}

//返回map里面的值
func Array_values(array map[string]string) []string {
	re := []string{}
	for _, v := range array {
		re = append(re, v)
	}
	return re
}

//去掉/../获取真实路径
func Realpath(path string) string {
	path_s := strings.Split(path, "/")
	realpath := []string{}
	if len(path_s) == 0 {
		return "error"
	}
	for _, value := range path_s {

		if value == ".." {
			k := len(realpath)
			kk := k - 1
			realpath = append(realpath[:kk], realpath[k:]...)
		} else {
			realpath = append(realpath, value)
		}
	}

	return strings.Join(realpath, "/")
}

//寻找中文的位置
func Chinese_str_index(str string, find string) int {
	rs := []rune(str)
	frs := []rune(find)
	index := -1
one:
	for k, v := range rs {
		if v == frs[0] {
			index = k
			for kk, vv := range frs {
				if k+kk > len(rs) || rs[k+kk] != vv {
					index = -1
					continue one
				}
			}
			return index
		}
	}
	return index
}

//删除最后一个切片，并返回值
func Array_pop(array *[]string) string {
	if *array == nil {
		return ""
	}
	length := len(*array)
	result := (*array)[length-1]
	*array = (*array)[:length-1]
	return result
}

//简单正则匹配,返回是否成功
func Preg_match(regtext string, text string) bool {

	r, err := regexp2.Compile(regtext, 0)
	if err != nil {
		Log("简单匹配正则语句出错%s", regtext)
		return false
	}

	m, err := r.FindStringMatch(text)
	if err != nil || m == nil {
		if err != nil {
			Log("简单匹配出错%v", err)
		}
		return false
	}
	return true
}

//返回匹配结果,n=次数
func Preg_match_result(regtext string, text string, n int) ([][]string, error) {

	r, err := regexp2.Compile(regtext, 0)
	if err != nil {
		return nil, err
	}

	m, err := r.FindStringMatch(text)
	if err != nil {
		return nil, err
	}
	var result [][]string
	for m != nil && n != 0 {
		var res_v []string
		for _, v := range m.Groups() {
			res_v = append(res_v, v.String())
		}

		m, _ = r.FindNextMatch(m)
		result = append(result, res_v)
		n--
	}

	return result, nil
}

//正则替换
func Preg_replace(regtext string, text string, src string) (string, error) {
	r, _ := regexp2.Compile(regtext, 0)
	return r.Replace(src, text, -1, -1)

}
func test() { fmt.Println() }

//组合map
func Array_merge(m ...map[string]interface{}) map[string]interface{} {
	if len(m) == 1 {
		return m[0]
	}
	result := make(map[string]interface{})
	for k, v := range m {
		if k > 0 {
			for key, val := range v {
				result[key] = val
			}
		} else {
			result = m[0]
		}
	}
	return result
}

//取字串符某几位
func Substr(str string, start, length int) string {
	rs := []rune(str)
	rl := len(rs)
	end := 0

	if start < 0 {
		start = rl - 1 + start
	}
	end = start + length

	if start > end {
		start, end = end, start
	}

	if start < 0 {
		start = 0
	}
	if start > rl {
		start = rl
	}
	if end < 0 {
		end = 0
	}
	if end > rl {
		end = rl
	}
	return string(rs[start:end])
}

//返回int64时间戳
func Timestampint() int64 {
	cur := time.Now()
	return cur.Unix()
}

//返回时间戳字串符
func Timestamp() string {
	cur := time.Now()
	return strconv.FormatInt(cur.Unix(), 10)
}

//微秒级时间戳
func Microtimeint() int64 {
	cur := time.Now()
	return cur.UnixNano() / 1000
}

//微秒级时间戳
func Microtime() string {
	cur := time.Now()
	return strconv.FormatInt(cur.UnixNano()/1000, 10)
}

//毫秒
func Millitimeint() int64 {
	cur := time.Now()
	return cur.UnixNano() / 1000000
}
func Millitime() string {
	cur := time.Now()
	return strconv.FormatInt(cur.UnixNano()/1000000, 10)
}

//获取指定目录下的所有文件，不进入下一级目录搜索，可以匹配后缀过滤。
func ListDir(dirPth string, suffix string) (files []string, err error) {
	files = make([]string, 0, 10)
	dir, err := ioutil.ReadDir(dirPth)
	if err != nil {
		return nil, err
	}
	PthSep := "/"
	if os.IsPathSeparator('\\') { //前边的判断是否是系统的分隔符
		PthSep = "\\"
	}
	suffix = strings.ToUpper(suffix) //忽略后缀匹配的大小写
	for _, fi := range dir {
		if fi.IsDir() { // 忽略目录
			continue
		}
		if strings.HasSuffix(strings.ToUpper(fi.Name()), suffix) { //匹配文件
			files = append(files, dirPth+PthSep+fi.Name())
		}
	}
	return files, nil
}

/*php函数之返回随机键名
 *第一个参数传入map或者切片
 *第二个参数传入返回数量，默认返回一个string
 */
func Array_rand(param ...interface{}) (result []string) {
	var num int64
	keys := []string{}
	if len(param) < 2 {
		num = 1
	} else {
		num = (int64)(param[1].(int))
	}
	if num < 1 {
		num = 1
	}
	switch param[0].(type) {
	case []interface{}:
		for k, _ := range param[0].([]interface{}) {
			keys = append(keys, strconv.Itoa(k))
		}
	case map[string]interface{}:
		for k, _ := range param[0].(map[string]interface{}) {
			keys = append(keys, k)
		}
	}
	var i int64
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i = 0; i < num; i++ {
		re := keys[r.Intn(len(keys))]
		Unset_ss(&keys, re)
		result = append(result, re)
	}
	return
}

//php unset函数变形之一,这里用于删除切片某个元素
func Unset_ss(src *[]string, dest string) {
	for i, v := range *src {
		if v != dest { //当V=-1时，假定是不需要的数据
			*src = append((*src)[:i], (*src)[i+1:]...)
		}
	}
}

//php array_unique函数之去除[]string重复
func Ss_unique(src *[]string) {
	result := false
end:
	for k1, v1 := range *src {
		for k2, v2 := range *src {
			if k1 == k2 || v2 == "" {
				continue
			}
			if v1 == v2 {
				*src = append((*src)[:k2], (*src)[k2+1:]...)
				result = true
				break end
			}
		}
	}
	if result {
		go Ss_unique(src)
	}
}

//php date函数,部分转换内容
func Date(format string, timestamp interface{}) string {
	t, _ := strconv.ParseInt(fmt.Sprint(timestamp), 10, 64)
	tm := time.Unix(t, 0)
	format = strings.Replace(format, "Y", "2006", 1)
	format = strings.Replace(format, "y", "06", 1)
	format = strings.Replace(format, "m", "01", 1)
	format = strings.Replace(format, "d", "02", 1)
	format = strings.Replace(format, "H", "15", 1)
	format = strings.Replace(format, "h", "03", 1)
	format = strings.Replace(format, "i", "04", 1)
	format = strings.Replace(format, "s", "05", 1)
	return tm.Format(format)
}

//删除重复的切片元素
func Split_unique(list *[]string) []string {
	var x []string = []string{}
	for _, i := range *list {
		if len(x) == 0 {
			x = append(x, i)
		} else {
			for k, v := range x {
				if i == v {
					break
				}
				if k == len(x)-1 {
					x = append(x, i)
				}
			}
		}
	}
	return x
}

//删除重复的数组值
func Map_unique(m *map[string]string) map[string]string {
	mm := make(map[string]string)
	for k, v := range *m {
		w := true
		for _, vv := range mm {
			if vv == v {
				w = false
				break
			}
		}
		if w {
			mm[k] = v
		}
	}
	return mm
}

//判断切片是否包含某元素
func In_slice(str interface{}, sp interface{}) bool {
	if sp == nil {
		return false
	}
	r := reflect.ValueOf(sp)
	for r.Kind() == reflect.Ptr {
		r = r.Elem()
	}
	if r.Kind() != reflect.Slice {
		return false
	}
	for i := 0; i < r.Len(); i++ {
		v := r.Index(i)
		if fmt.Sprint(str) == fmt.Sprint(v.Interface()) {
			return true
		}
	}
	return false
}

//把字串符转为指定小数点的字串符
func Number_format(s interface{}, d interface{}) string {
	var f float64
	var decimals string
	switch d.(type) {
	case string:
		decimals = d.(string)
	case int:
		decimals = strconv.Itoa(d.(int))
	case int64:
		decimals = strconv.FormatInt(d.(int64), 10)
	default:
		t := reflect.TypeOf(d)
		fmt.Println("Number_format decimals无法识别变量类型", t.Name())
	}

	switch s.(type) {
	case string:
		f, _ = strconv.ParseFloat(s.(string), 64)
	case int:
		f = float64(s.(int))
	case int32:
		f = float64(s.(int32))
	case int64:
		f = float64(s.(int64))
	case float32:
		f = float64(s.(float32))
	case float64:
		f = s.(float64)
	default:
		t := reflect.TypeOf(s)
		fmt.Println("Number_format float无法识别变量类型", t.Name())
	}

	return fmt.Sprintf("%."+decimals+"f", f)
}

func Round(s interface{}, places int) float64 {
	var val float64
	switch s.(type) {
	case string:
		val, _ = strconv.ParseFloat(s.(string), 64)
	case float64:
		val = s.(float64)
	case float32:
		val = float64(s.(float32))
	}
	var t float64
	f := math.Pow10(places)
	x := val * f
	if math.IsInf(x, 0) || math.IsNaN(x) {
		return val
	}
	if x >= 0.0 {
		t = math.Ceil(x)
		if (t - x) > 0.50000000001 {
			t -= 1.0
		}
	} else {
		t = math.Ceil(-x)
		if (t + x) > 0.50000000001 {
			t -= 1.0
		}
		t = -t
	}
	x = t / f

	if !math.IsInf(x, 0) {
		return x
	}

	return t
}

//对string进行html简单转义
func Html_encode(str string) string {
	str = strings.Replace(str, "&", "&amp;", -1)
	str = strings.Replace(str, `"`, "&quot;", -1)
	str = strings.Replace(str, "<", "&lt;", -1)
	str = strings.Replace(str, ">", "&gt;", -1)
	return str
}

//返回随机数
func Rand(start int, end int) int {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	n := r.Intn(end - start)
	return start + n
}

//js的>>>运算
func Shift_3(s interface{}, r uint8) int {
	var i32 int
	switch s.(type) {
	case int:
		i32 = s.(int)
	case int32:
		i32 = int(s.(int32))
	case int64:
		i32 = int(s.(int64))
	case uint8:
		i32 = int(s.(uint8))
	case uint64:
		i32 = int(s.(uint64))
	default:
		panic("未设置类型")
	}

	right := int(r)
	var n uint32 = uint32(i32)
	temp := []uint32{}
	for true {
		if n/2 >= 1 || n%2 == 1 {
			temp = append(temp, n%2)
			n = n / 2
		} else {
			break
		}
	}
	for i := 0; i < len(temp); i++ {
		if i+right >= len(temp) {
			temp[i] = 0
		} else {
			temp[i] = temp[i+right]
		}

	}
	n = 0
	var stnum, conum float64 = 0, 2
	for i := 0; i < len(temp); i++ {
		n += temp[i] * uint32(math.Pow(conum, stnum))
		stnum++
	}
	return int(n)
}
func CopyFile(srcName, dstName string) (written int64, err error) {

	src, err := os.Open(srcName)
	if err != nil {
		return
	}
	defer src.Close()
	dst, err := os.OpenFile(dstName, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return
	}
	defer dst.Close()
	return io.Copy(dst, src)
}

//处理Y-m-d H:i:s即2015-10-01 18:05:30
func Strtotime(str string) int64 {
	tm2, _ := time.ParseInLocation("2006-01-02 15:04:05", str, time.Local)
	return tm2.Unix()
}

func Reflect_setString(r *reflect.Value, s string) {
	switch r.Kind() {
	case reflect.Int, reflect.Int64:
		i, err := strconv.ParseInt(s, 10, 64)
		if err == nil {
			r.SetInt(i)
		}
	case reflect.Int32:
		i, err := strconv.ParseInt(s, 10, 32)
		if err == nil {
			r.SetInt(i)
		}
	case reflect.Int16:
		i, err := strconv.ParseInt(s, 10, 16)
		if err == nil {
			r.SetInt(i)
		}
	case reflect.Int8:
		i, err := strconv.ParseInt(s, 10, 8)
		if err == nil {
			r.SetInt(i)
		}
	case reflect.Uint, reflect.Uint64:
		i, err := strconv.ParseUint(s, 10, 64)
		if err == nil {
			r.SetUint(i)
		}
	case reflect.Uint32:
		i, err := strconv.ParseUint(s, 10, 32)
		if err == nil {
			r.SetUint(i)
		}
	case reflect.Uint16:
		i, err := strconv.ParseUint(s, 10, 16)
		if err == nil {
			r.SetUint(i)
		}
	case reflect.Uint8:
		i, err := strconv.ParseUint(s, 10, 8)
		if err == nil {
			r.SetUint(i)
		}
	case reflect.Float32:
		f, err := strconv.ParseFloat(s, 32)
		if err == nil {
			r.SetFloat(f)
		}
	case reflect.Float64:
		f, err := strconv.ParseFloat(s, 64)
		if err == nil {
			r.SetFloat(f)
		}
	case reflect.String:
		r.SetString(s)
	default:
		switch r.Interface().(type) {
		case []string:
			var tmp []string
			Msgpack_unpack(s, &tmp)
			r.Set(reflect.ValueOf(tmp))
		case map[string]string:
			var tmp map[string]string
			Msgpack_unpack(s, &tmp)
			r.Set(reflect.ValueOf(tmp))
		case map[string]map[string]string:
			var tmp map[string]map[string]string
			Msgpack_unpack(s, &tmp)
			r.Set(reflect.ValueOf(tmp))
		}
	}
}
func File_exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

//http_post
func Http_post(Url string, param ...map[string]string) (string, error) {

	var p []string
	if len(param) == 1 {
		p = make([]string, len(param[0]))
		var i int
		for key, val := range param[0] {
			p[i] = url.QueryEscape(key) + "=" + url.QueryEscape(val)
			i++
		}
	}
	reqest, err := http.NewRequest("POST", Url, strings.NewReader(strings.Join(p, "&"))) //建立一个请求
	if err != nil {
		return "", err
	}
	client := <-http_c
	defer func() {
		http_c <- client
	}()
	//Add 头协议
	reqest.Header.Add("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	reqest.Header.Add("Accept-Language", "ja,zh-CN;q=0.8,zh;q=0.6")
	reqest.Header.Add("Connection", "keep-alive")
	reqest.Header.Add("User-Agent", "Mozilla/5.0 (Windows NT 6.1; WOW64; rv:12.0) Gecko/20100101 Firefox/12.0")
	response, err := client.Do(reqest) //提交
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	/*cookies := response.Cookies() //遍历cookies
	for _, cookie := range cookies {
		fmt.Println("cookie:", cookie)
	}*/

	body, err1 := ioutil.ReadAll(response.Body)
	if err1 != nil {
		return "", err1
	}
	return string(body), nil
}

//get方法
func Http_get(Url string, param ...map[string]string) (string, error) {
	var p []string
	if len(param) == 1 {
		p = make([]string, len(param[0]))
		var i int
		for key, val := range param[0] {
			p[i] = url.QueryEscape(key) + "=" + url.QueryEscape(val)
		}
	}
	if strings.Index(Url, "?") > -1 {
		Url += "&" + strings.Join(p, "&")
	} else {
		Url += "?" + strings.Join(p, "&")
	}
	reqest, err := http.NewRequest("GET", Url, nil) //建立一个请求
	if err != nil {
		return "", err
	}
	client := <-http_c
	defer func() {
		http_c <- client
	}()
	//Add 头协议
	reqest.Header.Add("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	reqest.Header.Add("Accept-Language", "ja,zh-CN;q=0.8,zh;q=0.6")
	reqest.Header.Add("Connection", "keep-alive")
	reqest.Header.Add("User-Agent", "Mozilla/5.0 (Windows NT 6.1; WOW64; rv:12.0) Gecko/20100101 Firefox/12.0")
	response, err := client.Do(reqest) //提交
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	/*cookies := response.Cookies() //遍历cookies
	for _, cookie := range cookies {
		fmt.Println("cookie:", cookie)
	}*/
	body, err1 := ioutil.ReadAll(response.Body)
	if err1 != nil {
		return "", err1
	}
	return string(body), nil
}

func Http2file(Url string, filename string) error {
	reqest, err := http.NewRequest("GET", Url, nil) //建立一个请求
	if err != nil {
		return err
	}
	client := <-http_c
	defer func() {
		http_c <- client
	}()
	//Add 头协议
	reqest.Header.Add("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	reqest.Header.Add("Accept-Language", "ja,zh-CN;q=0.8,zh;q=0.6")
	reqest.Header.Add("Connection", "keep-alive")
	reqest.Header.Add("User-Agent", "Mozilla/5.0 (Windows NT 6.1; WOW64; rv:12.0) Gecko/20100101 Firefox/12.0")
	response, err := client.Do(reqest) //提交
	if err != nil {
		return err
	}
	defer response.Body.Close()
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	body, err1 := ioutil.ReadAll(response.Body)
	if err1 != nil {
		return err1
	}
	_, err = f.Write(body)
	return err
}

// 判断文件夹是否存在
func PathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
func SearchInt32(src []int32, dst int32) int32 {
	if len(src) == 0 || dst < src[0] || dst > src[len(src)-1] {
		return -1
	}
	var begin, end, _dst, k int32
	end = int32(len(src)) - 1
	for true {
		if end-begin == 1 {
			if src[end] == dst {
				return end
			} else if src[begin] == dst {
				return begin
			}
			return -1
		}
		k = (end-begin)/2 + begin
		_dst = src[k]
		switch {
		case _dst > dst:
			end = k
		case _dst < dst:
			begin = k
		case _dst == dst:
			return k
		}
	}
	return 0
}
func Search(src []int, dst int) int {
	if len(src) == 0 || dst < src[0] || dst > src[len(src)-1] {
		return -1
	}
	var begin, end, _dst, k int
	end = len(src) - 1
	for true {
		if end-begin == 1 {
			if src[end] == dst {
				return end
			} else if src[begin] == dst {
				return begin
			}
			return -1
		}
		k = (end-begin)/2 + begin
		_dst = src[k]
		switch {
		case _dst > dst:
			end = k
		case _dst < dst:
			begin = k
		case _dst == dst:
			return k
		}
	}
	return 0
}
func Search_egt(src []int, dst int) int {
	if len(src) == 0 || dst < src[0] || dst > src[len(src)-1] {
		return -1
	}
	var begin, end, _dst, k int
	end = len(src) - 1
	for true {
		if end-begin == 1 {
			if src[begin] == dst {
				return begin
			}
			return end
		}
		k = (end-begin)/2 + begin
		_dst = src[k]
		switch {
		case _dst > dst:
			end = k
		case _dst < dst:
			begin = k
		case _dst == dst:
			return k
		}
	}
	return 0
}
func Search_egt32(src []int32, dst int32) int32 {
	if len(src) == 0 || dst < src[0] || dst > src[len(src)-1] {
		return -1
	}
	var begin, end, _dst, k int32
	end = int32(len(src)) - 1
	for true {
		if end-begin == 1 {
			if src[begin] == dst {
				return begin
			}
			return end
		}
		k = (end-begin)/2 + begin
		_dst = src[k]
		switch {
		case _dst > dst:
			end = k
		case _dst < dst:
			begin = k
		case _dst == dst:
			return k
		}
	}
	return 0
}
func Search_elt(src []int, dst int) int {
	if len(src) == 0 || dst < src[0] || dst > src[len(src)-1] {
		return -1
	}
	var begin, end, _dst, k int
	end = len(src) - 1
	for true {
		if end-begin == 1 {
			if src[end] == dst {
				return end
			}
			return begin
		}
		k = (end-begin)/2 + begin
		_dst = src[k]
		switch {
		case _dst > dst:
			end = k
		case _dst < dst:
			begin = k
		case _dst == dst:
			return k
		}
	}
	return 0
}
func Search_elt32(src []int32, dst int32) int32 {
	if len(src) == 0 || dst < src[0] || dst > src[len(src)-1] {
		return -1
	}
	var begin, end, _dst, k int32
	end = int32(len(src)) - 1
	for true {
		if end-begin == 1 {
			if src[end] == dst {
				return end
			}
			return begin
		}
		k = (end-begin)/2 + begin
		_dst = src[k]
		switch {
		case _dst > dst:
			end = k
		case _dst < dst:
			begin = k
		case _dst == dst:
			return k
		}
	}
	return 0
}
func Search_float32(src []float32, dst float32) int {
	if len(src) == 0 || dst < src[0] || dst > src[len(src)-1] {
		return -1
	}
	var begin, end, k int
	var _dst float32
	end = len(src) - 1
	for true {
		if end-begin == 1 {
			if src[end] == dst {
				return end
			} else if src[begin] == dst {
				return begin
			}
			return -1
		}
		k = (end-begin)/2 + begin
		_dst = src[k]
		switch {
		case _dst > dst:
			end = k
		case _dst < dst:
			begin = k
		case _dst == dst:
			return k
		}
	}
	return 0
}
func Search_float64(src []float64, dst float64) int {
	if len(src) == 0 || dst < src[0] || dst > src[len(src)-1] {
		return -1
	}
	var begin, end, k int
	var _dst float64
	end = len(src) - 1
	for true {
		if end-begin == 1 {
			if src[end] == dst {
				return end
			} else if src[begin] == dst {
				return begin
			}
			return -1
		}
		k = (end-begin)/2 + begin
		_dst = src[k]
		switch {
		case _dst > dst:
			end = k
		case _dst < dst:
			begin = k
		case _dst == dst:
			return k
		}
	}
	return 0
}
func Search_float64_egt(src []float64, dst float64) int {
	if len(src) == 0 || dst < src[0] || dst > src[len(src)-1] {
		return -1
	}
	var begin, end, k int
	var _dst float64
	end = len(src) - 1
	for true {
		if end-begin == 1 {
			if src[begin] == dst {
				return begin
			}
			return end
		}
		k = (end-begin)/2 + begin
		_dst = src[k]
		switch {
		case _dst > dst:
			end = k
		case _dst < dst:
			begin = k
		case _dst == dst:
			return k
		}
	}
	return 0
}
func Search_float64_elt(src []float64, dst float64) int {
	if len(src) == 0 || dst < src[0] || dst > src[len(src)-1] {
		return -1
	}
	var begin, end, k int
	var _dst float64
	end = len(src) - 1
	for true {
		if end-begin == 1 {
			if src[end] == dst {
				return end
			}
			return begin
		}
		k = (end-begin)/2 + begin
		_dst = src[k]
		switch {
		case _dst > dst:
			end = k
		case _dst < dst:
			begin = k
		case _dst == dst:
			return k
		}
	}
	return 0
}
func GoRoutineExecFunction(f func()) {
	go func() {
		defer func() { // 必须要先声明defer，否则不能捕获到panic异常
			if err := recover(); err != nil {
				stack := debug.Stack()
				Log("%v\n%s", err, Bytes2str(stack))
			}
		}()
		f()
	}()
}

func SortInt(list []int) {
	var step, l, max, r, r_b, index, n int
	max_len := len(list)
	tmp := make([]int, max_len)

	step = 1
	for {
		n++
		step <<= 1
		if n&1 == 1 {
			for i := 0; i < max_len; i += step {
				l = i
				max = i + step
				r = (max-l)/2 + l
				r_b = r
				if max > max_len {
					max = max_len
				}
				index = i
				for index < max {
					switch {
					case r >= max:
						tmp[index] = list[l]
						l++
					case l == r_b:
						tmp[index] = list[r]
						r++
					case list[l] > list[r]:
						tmp[index] = list[r]
						r++
					default:
						tmp[index] = list[l]
						l++
					}
					index++
				}

			}
		} else {
			for i := 0; i < max_len; i += step {
				l = i
				max = i + step
				r = (max-l)/2 + l
				r_b = r
				if max > max_len {
					max = max_len
				}
				index = i
				for index < max {
					switch {
					case r >= max:
						list[index] = tmp[l]
						l++
					case l == r_b:
						list[index] = tmp[r]
						r++
					case tmp[l] > tmp[r]:
						list[index] = tmp[r]
						r++
					default:
						list[index] = tmp[l]
						l++
					}
					index++
				}

			}
		}
		if step > max_len {
			if n&1 == 1 {
				copy(list, tmp)
			}
			return
		}
	}
}

func SortInt32(list []int32) {
	max_len := len(list)
	for i := 0; i < max_len-max_len&1; i += 2 {
		if list[i] > list[i+1] {
			list[i], list[i+1] = list[i+1], list[i]
		}

	}
	for i := 0; i < max_len-max_len&3; i += 4 {
		if list[i] > list[i+2] {
			list[i], list[i+2] = list[i+2], list[i]
		}
		if list[i+1] > list[i+3] {
			list[i+1], list[i+3] = list[i+3], list[i+1]
		}
		if list[i+1] > list[i+2] {
			list[i+1], list[i+2] = list[i+2], list[i+1]
		}

	}
	if max_len&3 == 3 {
		i := max_len - 3
		if list[i] > list[i+2] {
			list[i+1], list[i+2] = list[i+2], list[i+1]
			list[i], list[i+1] = list[i+1], list[i]
		} else if list[i+1] > list[i+2] {
			list[i+1], list[i+2] = list[i+2], list[i+1]
		}
	}
	var step, l, max, r, r_b, index, n int
	tmp := make([]int32, max_len)
	step = 4
	for step < max_len {
		n++
		step <<= 1
		if n&1 == 1 {
			for i := 0; i < max_len; i += step {
				l = i
				max = i + step
				r = (max-l)/2 + l
				r_b = r
				if max > max_len {
					max = max_len
				}
				index = i
				for index < max {
					switch {
					case r >= max:
						tmp[index] = list[l]
						l++
					case l == r_b:
						tmp[index] = list[r]
						r++
					case list[l] > list[r]:
						tmp[index] = list[r]
						r++
					default:
						tmp[index] = list[l]
						l++
					}
					index++
				}

			}
		} else {
			for i := 0; i < max_len; i += step {
				l = i
				max = i + step
				r = (max-l)/2 + l
				r_b = r
				if max > max_len {
					max = max_len
				}
				index = i
				for index < max {
					switch {
					case r >= max:
						list[index] = tmp[l]
						l++
					case l == r_b:
						list[index] = tmp[r]
						r++
					case tmp[l] > tmp[r]:
						list[index] = tmp[r]
						r++
					default:
						list[index] = tmp[l]
						l++
					}
					index++
				}

			}
		}
	}
	if n&1 == 1 {
		copy(list, tmp)
	}
}
func MatchInt32(less_i, more_i []int32) []int32 {
	result := make([]int32, len(less_i))
	var index, l, m int //结果，less，more的指针
	en_l := len(less_i)
	en_m := len(more_i)
	for true {
		if l == en_l || m == en_m { //指针到头
			break
		}
		switch {
		case less_i[l] == more_i[m]:
			result[index] = less_i[l]
			index++
			l++
			m++
		case less_i[l] > more_i[m]:
			m++
		default:
			l++
		}
	}
	return result[:index]
}

func Todaytimestamp() int64 {
	return Todaytime().Unix()
}
func Todaytime() time.Time {
	timeStr := time.Now().Format("2006-01-02")
	tm2, _ := time.ParseInLocation("2006-01-02", timeStr, time.Local)
	return tm2
}
func SortAny(i interface{}, f func(interface{}, interface{}) bool) {
	ref := reflect.ValueOf(i)
	if ref.Kind() != reflect.Slice {
		return
	}
	ref2 := reflect.MakeSlice(ref.Type(), ref.Len(), ref.Len())
	reflect.Copy(ref2, ref)
	max_len := ref.Len()
	step := 1
	var n, l, max, r, r_b, index int
	for true {
		n++
		step *= 2
		if n%2 == 1 {
			for i := 0; i < max_len; i += step {
				l = i
				max = i + step
				r = (max-l)/2 + l
				r_b = r
				if max > max_len {
					max = max_len
				}
				max--
				index = i - 1 //直接用index=i计算错误，原因未知

				for index < max {
					index++
					switch {
					case r > max:
						ref2.Index(index).Set(ref.Index(l))
						l++
					case l == r_b:
						ref2.Index(index).Set(ref.Index(r))
						r++
					case f(ref.Index(l).Interface(), ref.Index(r).Interface()):
						ref2.Index(index).Set(ref.Index(r))
						r++
					default:
						ref2.Index(index).Set(ref.Index(l))
						l++
					}
				}

			}
		} else {
			for i := 0; i < max_len; i += step {
				l = i
				max = i + step
				r = (max-l)/2 + l
				r_b = r
				if max > max_len {
					max = max_len
				}
				max--
				index = i - 1 //直接用index=i计算错误，原因未知

				for index < max {
					index++
					switch {
					case r > max:
						ref.Index(index).Set(ref2.Index(l))

						l++
					case l == r_b:
						ref.Index(index).Set(ref2.Index(r))
						r++
					case f(ref2.Index(l).Interface(), ref2.Index(r).Interface()):
						ref.Index(index).Set(ref2.Index(r))
						r++
					default:
						ref.Index(index).Set(ref2.Index(l))
						l++
					}
				}

			}
		}
		if step > max_len {
			if n%2 == 1 {
				reflect.Copy(ref, ref2)
			}
			return
		}
	}
}
func Str2bytes(s string) []byte {
	x := (*[2]uintptr)(unsafe.Pointer(&s))
	h := [3]uintptr{x[0], x[1], x[1]}
	return *(*[]byte)(unsafe.Pointer(&h))
}

func Bytes2str(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}
func DEBUG(v ...interface{}) {

	_, file, line, ok := runtime.Caller(1)
	if ok {
		v = append([]interface{}{fmt.Sprintf("%s,line %d:", file, line)}, v...)
	}
	fmt.Println(v...)

}
func Log(format string, v ...interface{}) {
	_, file, line, ok := runtime.Caller(1)
	if ok {
		v = append([]interface{}{fmt.Sprintf("%s,line %d:", file, line)}, v...)
	}
	fmt.Printf("%s "+format+"\r\n", v...)
}

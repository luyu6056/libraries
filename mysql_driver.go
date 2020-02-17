package libraries

import (
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"runtime/debug"
	"sync/atomic"

	//"fmt"

	"io/ioutil"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/panjf2000/ants"
)

const (
	max_packet_size      = 16777216
	Default_MaxOpenConns = 20 //默认连接数
	Default_MaxIdleConns = 10 //默认空链接
)

type MysqlDB struct {
	username           string
	passwd             string
	database           string
	ip_port            string
	clientCharsetIndex uint8
	MaxOpenConns       int32            //最大连接数
	MaxIdleConns       int32            //最大空链接
	Conn_num           int32            //连接数量统计
	ConnMaxLifetime    int64            //最大连接时间
	Conn_chan          chan *Mysql_Conn //连接池
	Conn_chan2         chan *Mysql_Conn //连接池
	//Ping_chan          chan *Mysql_Conn //ping的中转chan
	Conn_m sync.Map //map类型池
	Lock   sync.Mutex
	gopool *ants.Pool
}

type Mysql_Conn struct {
	//Version            string
	Thread_id uint32 //线程ID
	//seed               []byte
	//serverCapabilities uint16 //协议协商
	//serverCharsetIndex uint8  //编码格式
	//clientCharsetIndex uint8
	//serverStatus       uint16 //状态码
	//restOfScrambleBuff []byte //保留字节 长度13
	//seed2 []byte //长度12
	//capability_flags   uint16
	//auth_plugin_name   string
	Status bool
	//username      string
	//passwd        string
	//database      string
	msg_no uint8
	conn   net.Conn
	DB     *MysqlDB
	buffer *MsgBuffer
	//msg_buffer_no int

	//Debug         bool
	//buf_4 []byte
	//wg           sync.WaitGroup
	pingtime int64
	Lock     sync.Mutex
	istx     bool
}

var pingadd = int64(300) //300秒ping一次

func mysql_open(user, passwd, ip_port, database, charset string) *MysqlDB {
	db := &MysqlDB{
		username:           user,
		passwd:             passwd,
		ip_port:            ip_port,
		database:           database,
		clientCharsetIndex: collations[charset],
	}
	if db.clientCharsetIndex == 0 {
		db.clientCharsetIndex = 33
	}
	return db
}
func (mysqldb *MysqlDB) Ping() error {
	if mysqldb.MaxOpenConns < 1 {
		mysqldb.MaxOpenConns = Default_MaxOpenConns
	}
	if mysqldb.MaxIdleConns < 1 {
		mysqldb.MaxIdleConns = Default_MaxIdleConns
	}
	if mysqldb.MaxIdleConns > mysqldb.MaxOpenConns {
		mysqldb.MaxIdleConns = mysqldb.MaxOpenConns
	}
	mysqldb.Conn_chan = make(chan *Mysql_Conn, mysqldb.MaxIdleConns)
	mysqldb.Conn_chan2 = make(chan *Mysql_Conn, mysqldb.MaxOpenConns) //预留一些用作ping
	//mysqldb.Ping_chan = make(chan *Mysql_Conn, mysqldb.MaxOpenConns*2) //多加点避免ping阻塞
	mysqldb.gopool, _ = ants.NewPool(int(mysqldb.MaxOpenConns * 2)) //多加点避免ping阻塞正常查询

	var err error

	for i := int32(0); i < mysqldb.MaxIdleConns; i++ {
		var conn *Mysql_Conn
		conn, err = mysqldb.connect_new()
		if err != nil || conn == nil {
			continue
		}
		if conn.Status != true {
			continue
		}
		mysqldb.Conn_chan <- conn
		mysqldb.Conn_num++
		mysqldb.Conn_m.Store(conn.Thread_id, conn)

	}

	if len(mysqldb.Conn_chan) == 0 {
		return errors.New("无法创建数据库连接," + err.Error())
	}
	var status = true
	ping := func() {
		defer func() {
			if err := recover(); err != nil {
				fmt.Println(err)
				debug.PrintStack()
			}
		}()
		now := time.Now().Unix()
		if mysqldb == nil {
			status = false
			return
		}
		mysqldb.Conn_m.Range(func(key, v interface{}) bool {
			if v == nil {
				DEBUG("致命错误,range出现nil")
				mysqldb.Lock.Lock()
				if _, ok := mysqldb.Conn_m.Load(key); ok {
					mysqldb.Conn_m.Delete(key)
					atomic.AddInt32(&mysqldb.Conn_num, -1)
				}
				mysqldb.Lock.Unlock()
			}
			return true
		})
	pingfor:
		for {
			var conn *Mysql_Conn
			var conn2List []*Mysql_Conn
			select {
			case conn = <-mysqldb.Conn_chan: //优先处理Conn1并且把结果放到conn2去
				if conn = conn.ping(now); conn != nil {
					select {
					case mysqldb.Conn_chan2 <- conn:
					default:
						conn2List = append(conn2List, conn) //conn2满了，放到临时列表
					}
				}
			default:
				for {
					select {
					case conn = <-mysqldb.Conn_chan2: //从conn2拿出来，不会二次ping
						if conn = conn.ping(now); conn != nil {
							select {
							case mysqldb.Conn_chan <- conn: //优先放回conn
							default:
								conn2List = append(conn2List, conn)
							}
						}
					default:
						for _, conn := range conn2List {
							mysqldb.Put(conn) //最终再丢回去
						}
						break pingfor
					}

				}
				break pingfor
			}

		}

		if mysqldb.Conn_num < mysqldb.MaxIdleConns {
			mysqldb.gopool.Submit(func() {
				for i := int32(0); i < mysqldb.MaxIdleConns; i++ { //避免连接失败一直重试
					conn, err := mysqldb.connect_new()
					if err == nil && conn != nil && conn.Status {
						mysqldb.Conn_m.Store(conn.Thread_id, conn)
						atomic.AddInt32(&mysqldb.Conn_num, 1)
						select {
						case mysqldb.Conn_chan <- conn:
						case mysqldb.Conn_chan2 <- conn:
						default:
							DEBUG("发生致命错误,mysql conn_num不足，无法新增入库")
							mysqldb.Conn_m.Delete(conn.Thread_id)
							atomic.AddInt32(&mysqldb.Conn_num, -1)
						}

					}
					if mysqldb.Conn_num >= mysqldb.MaxIdleConns {
						break
					}
				}
				if mysqldb.Conn_num == 0 {
					DEBUG("mysqldb无法创建连接") //断网?
				}
			})

		}

	}
	go func() {
		for status {
			time.Sleep(time.Second * 10)
			mysqldb.gopool.Submit(ping)

		}
	}()
	return nil
}
func (conn *Mysql_Conn) ping(now int64) *Mysql_Conn {

	if conn.Status == false {
		conn.Status = false
		conn.DB.Lock.Lock()
		if _, ok := conn.DB.Conn_m.Load(conn.Thread_id); ok {
			conn.DB.Conn_m.Delete(conn.Thread_id)
			atomic.AddInt32(&conn.DB.Conn_num, -1)
		}
		conn.DB.Lock.Unlock()
		conn.Close()
		return nil
	}

	if now > conn.pingtime {
		conn.msg_no = 255
		err := conn.writemsg([]byte{0xe})
		if err != nil {
			conn.Status = false
			conn.Close()
			conn.DB.Lock.Lock()
			if _, ok := conn.DB.Conn_m.Load(conn.Thread_id); ok {
				conn.DB.Conn_m.Delete(conn.Thread_id)
				atomic.AddInt32(&conn.DB.Conn_num, -1)
			}
			conn.DB.Lock.Unlock()
			return nil
		}
		conn.buffer.Reset()
		_, _, result, _, err := conn.readmsg()
		if err != nil || result != 0 { //非ok报文或者出错
			conn.Status = false
			conn.Close()
			conn.DB.Lock.Lock()
			if _, ok := conn.DB.Conn_m.Load(conn.Thread_id); ok {
				conn.DB.Conn_m.Delete(conn.Thread_id)
				atomic.AddInt32(&conn.DB.Conn_num, -1)
			}
			conn.DB.Lock.Unlock()
			return nil
		}
		conn.pingtime += pingadd
	}
	return conn
}
func (mysql *Mysql_Conn) Close() {
	if mysql != nil && mysql.conn != nil {
		mysql.buffer.Reset()
		b := mysql.buffer.Make(5)
		b[0] = 1
		b[1] = 0
		b[2] = 0
		b[3] = 0
		b[4] = 1 //COM_QUIT
		mysql.buffer.WriteByte(0)
		mysql.buffer.WriteByte(0x01)
		mysql.conn.Write(mysql.buffer.Bytes())

		mysql.conn.Close()
		mysql.conn = nil
	}
	mysql.Status = false
}
func (mysqldb *MysqlDB) connect_new() (*Mysql_Conn, error) {
	now := time.Now().Unix()
	var new_connect = &Mysql_Conn{
		buffer:   new(MsgBuffer),
		DB:       mysqldb,
		pingtime: now + rand.Int63n(pingadd), //避免集中ping
	}
	var conn net.Conn
	if strings.Contains(mysqldb.ip_port, ".sock") {
		addr, err := net.ResolveUnixAddr("unix", mysqldb.ip_port[:strings.Index(mysqldb.ip_port, ".sock")+5])
		if err != nil {
			return nil, err
		}
		conn, err = net.DialUnix("unix", nil, addr)
		if err != nil {
			return nil, err
		}
	} else {
		tcpAddr, err := net.ResolveTCPAddr("tcp4", mysqldb.ip_port)
		if err != nil {
			return nil, err
		}
		conn, err = net.DialTCP("tcp", nil, tcpAddr)

		if err != nil {
			return nil, err
		}
	}

	new_connect.conn = conn
	//new_connect.buf_4 = make([]byte, 4)
	//new_connect.buf_exec = []byte{0, 0, 0, 0, 3}
	err, seed, seed2 := new_connect.handshakePacket()
	if err != nil {
		return nil, err
	}
	new_connect.handshakeResponse(seed, seed2)
	return new_connect, nil
}
func (mysqldb *MysqlDB) GET() (conn *Mysql_Conn, err error) {
Loop:
	for conn == nil {
		select {
		case conn = <-mysqldb.Conn_chan:
			if conn == nil || !conn.Status {
				conn = nil
				DEBUG("从正常池子取出问题conn")
				Log("%+v", conn)
				continue Loop
			}
		case conn = <-mysqldb.Conn_chan2:
			if conn == nil || !conn.Status {
				conn = nil
				DEBUG("从池子2取出问题conn")
				Log("%+v", conn)
				continue Loop
			}
		default: //缓冲为空尝试新建
			if mysqldb.Conn_num < mysqldb.MaxOpenConns {
				atomic.AddInt32(&mysqldb.Conn_num, 1)
				conn, err = mysqldb.connect_new()
				if err == nil && conn != nil && conn.Status {
					mysqldb.Conn_m.Store(conn.Thread_id, conn)
				} else {
					atomic.AddInt32(&mysqldb.Conn_num, -1)
					conn = nil
					continue Loop
				}
				//DEBUG("创建连接")
			} else { //连接已满，强制等待
				select {
				case conn = <-mysqldb.Conn_chan:
					if conn == nil || !conn.Status {
						conn = nil
						continue Loop
					}
				case conn = <-mysqldb.Conn_chan2:
					if conn == nil || !conn.Status {
						conn = nil
						continue Loop
					}
				}
			}
		}
		if conn == nil {
			DEBUG("取出空的连接")
			err = errors.New("取出空的mysql连接")
			return
		}
	}
	//DEBUG("GET", conn.Thread_id)
	return
}
func (mysqldb *MysqlDB) Query(sql []byte, row *MysqlRows) (columns [][]byte, err error) {
	conn, err := mysqldb.GET()
	if err != nil {
		return nil, err
	}
	if conn == nil || conn.Status == false {
		DEBUG("取出问题conn")
	}
	columns, err = conn.Query(sql, row)
	mysqldb.Put(conn)
	return
}
func (mysqldb *MysqlDB) Exec(sql []byte) (int64, int64, error) {
	conn, err := mysqldb.GET()
	if err != nil {
		return 0, 0, err
	}
	if conn == nil || conn.Status == false {
		DEBUG("取出问题conn")
	}
	l, r, err := conn.Exec(sql)
	mysqldb.Put(conn)
	return l, r, err
}

var start_transaction = []byte{115, 116, 97, 114, 116, 32, 116, 114, 97, 110, 115, 97, 99, 116, 105, 111, 110}

func (mysqldb *MysqlDB) Begin() (*Mysql_Conn, error) {
	conn, err := mysqldb.connect_new()
	if err != nil {
		return nil, err
	}
	conn.istx = true
	_, _, err = conn.Exec(start_transaction) //start transaction
	if err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func (mysqldb *MysqlDB) Put(conn *Mysql_Conn) {
	//DEBUG("Put", conn.Thread_id)
	if mysqldb == nil || conn == nil || conn.istx {
		DEBUG("mysqldb", mysqldb)
		DEBUG("conn", conn)
		return
	}
	if conn.Status {
		if _, ok := mysqldb.Conn_m.LoadOrStore(conn.Thread_id, conn); !ok {
			atomic.AddInt32(&mysqldb.Conn_num, 1)
		}
		select {
		case mysqldb.Conn_chan <- conn:
		default: //缓冲池已满
			select {
			case mysqldb.Conn_chan2 <- conn:
			default: ////两个池都满 不大可能
				DEBUG("池子总量", mysqldb.Conn_num)
				conn.Status = false
				mysqldb.Lock.Lock()
				if _, ok := mysqldb.Conn_m.Load(conn.Thread_id); ok {
					mysqldb.Conn_m.Delete(conn.Thread_id)
					atomic.AddInt32(&mysqldb.Conn_num, -1)
				}
				mysqldb.Lock.Unlock()
				conn.Close()
				DEBUG("池子总量", mysqldb.Conn_num)
			}
		}
	} else {
		mysqldb.Lock.Lock()
		if _, ok := mysqldb.Conn_m.Load(conn.Thread_id); ok {
			mysqldb.Conn_m.Delete(conn.Thread_id)
			atomic.AddInt32(&mysqldb.Conn_num, -1)
		}
		mysqldb.Lock.Unlock()
		conn.Close()
	}

}

func (mysql *Mysql_Conn) Query(sql []byte, rows *MysqlRows) (columns [][]byte, err error) {
	msglen := len(sql) + 1
	if msglen > max_packet_size {
		err = errors.New("消息大于最大长度" + strconv.Itoa(max_packet_size))
		return
	}
	mysql.buffer.Reset()
	b := mysql.buffer.Make(5)
	b[0] = byte(msglen)
	b[1] = byte(msglen >> 8)
	b[2] = byte(msglen >> 16)
	b[3] = 0
	b[4] = 3

	//DEBUG(string(sql))
	mysql.buffer.Write(sql)
	_, err = mysql.conn.Write(mysql.buffer.Bytes())
	if err != nil {
		if strings.Contains(err.Error(), "connection reset by peer") {
			err = errors.New("EOF")
		}
		mysql.Status = false
		mysql.Close()
		return
	}
	mysql.buffer.Reset()
	//mysql.msg_buffer_no = 0
	var errmsg string
	_, _, rows.field_len, errmsg, err = mysql.readmsg()
	if errmsg != "" {
		if strings.Contains(errmsg, "1927-Connection was killed") {
			err = errors.New("EOF")
		} else { //err报文不影响mysql的status,在这里重新包装err
			err = errors.New(errmsg)
			return
		}
	}
	if err != nil {
		mysql.Status = false
		mysql.Close()
		return
	}
	columns, err = rows.Columns(mysql)
	if err != nil {
		mysql.Status = false
		mysql.Close()
		return
	}
	//mysql.mysqlRows.msg_no = mysql.msg_no
	//DEBUG(mysql.buffer.Bytes())
	return columns, nil
}
func (mysql *Mysql_Conn) Exec(sql []byte) (lastInsertId int64, rowsAffected int64, err error) {
	msglen := len(sql) + 1
	if msglen > max_packet_size {
		err = errors.New("消息大于最大长度" + strconv.Itoa(max_packet_size))
		return
	}
	mysql.buffer.Reset()
	b := mysql.buffer.Make(5)
	b[0] = byte(msglen)
	b[1] = byte(msglen >> 8)
	b[2] = byte(msglen >> 16)
	b[3] = 0
	b[4] = 3
	mysql.buffer.Write(sql)
	//DEBUG(string(sql))
	_, err = mysql.conn.Write(mysql.buffer.Bytes())
	if err != nil {
		if strings.Contains(err.Error(), "connection reset by peer") {
			err = errors.New("EOF")
		}
		mysql.Status = false
		mysql.Close()
		return
	}
	mysql.buffer.Reset()
	//mysql.msg_buffer_no = 0
	rowsAffected, lastInsertId, _, errmsg, err := mysql.readmsg()
	if errmsg != "" {
		if strings.Contains(errmsg, "1927-Connection was killed") {
			err = errors.New("EOF")
		} else { //err报文不影响mysql的status,在这里重新包装err
			err = errors.New(errmsg)
			return
		}
	}
	if err != nil {
		mysql.Status = false
		mysql.Close()
		return
	}
	return
}

//一次性读取n个字节
const mysqlmsglen = 1024 * 1024

func (mysql *Mysql_Conn) read() error {
	buf := mysql.buffer.Make(mysqlmsglen)
	n, err := mysql.conn.Read(buf)

	if n < mysqlmsglen {
		mysql.buffer.Truncate(mysql.buffer.Len() - (mysqlmsglen - n))
	}
	return err
}

var commit = []byte{99, 111, 109, 109, 105, 116}

func (mysql *Mysql_Conn) Commit() error {
	_, _, err := mysql.Exec(commit)
	return err
}

var rollback = []byte{114, 111, 108, 108, 98, 97, 99, 107}

func (mysql *Mysql_Conn) Rollback() error {
	_, _, err := mysql.Exec(rollback)
	return err
}

//握手包
func (mysql *Mysql_Conn) handshakePacket() (err error, seed []byte, seed2 []byte) {
	conn := mysql.conn
	mysql.buffer.Reset()
	msglen, err := mysql.readOneMsg()
	if msglen < 1 || err != nil {
		err = errors.New(conn.RemoteAddr().String() + "连接数据库失败，获取消息报文长度错误")
		return
	}
	if mysql.buffer.Next(1)[0] != 10 {
		err = errors.New(conn.RemoteAddr().String() + "连接数据库失败,不支持的协议版本")
		return
	}

	//mysql.Version, err =
	ReadNullTerminatedString(mysql.buffer)
	mysql.Thread_id = binary.LittleEndian.Uint32(mysql.buffer.Next(4))

	seed = make([]byte, 8)
	_, err = mysql.buffer.Read(seed)

	if err != nil {
		err = errors.New(conn.RemoteAddr().String() + "连接数据库失败,无法获取seed")
		return
	}

	mysql.buffer.ReadByte() //读取0x00
	//mysql.serverCapabilities = binary.LittleEndian.Uint16()
	//mysql.buffer.Next(8)
	//mysql.buffer.Read(mysql.buf_4[:1])
	//mysql.serverCharsetIndex = mysql.buf_4[0]
	//mysql.serverStatus = binary.LittleEndian.Uint16(mysql.buffer.Next(2))

	//new_connect.restOfScrambleBuff = make([]byte, 13)
	//binary.Read(mysql.buffer, binary.LittleEndian, &mysql.capability_flags)

	mysql.buffer.Shift(7)
	x, _ := mysql.buffer.ReadByte()

	if x == 0 {
		return
	}
	mysql.buffer.Shift(10) //读取10个字节
	seed2 = make([]byte, 12)
	mysql.buffer.Read(seed2)

	//mysql.buffer.Read(make([]byte, 1)) //读取0x00
	//mysql.auth_plugin_name, _ = ReadNullTerminatedString(mysql.buffer)
	//reader.Read(new_connect.restOfScrambleBuff)
	return
}
func (mysql *Mysql_Conn) handshakeResponse(seed, seed2 []byte) {
	mysql.buffer.Reset()
	//binary.Write(reader, binary.LittleEndian, uint32(new_connect.capability_flags))
	mysql.buffer.Write([]byte{141, 162, 10, 0})

	binary.LittleEndian.PutUint32(mysql.buffer.Make(4), uint32(max_packet_size))
	mysql.buffer.WriteByte(mysql.DB.clientCharsetIndex)

	mysql.buffer.Make(23)
	WriteNullTerminatedString(mysql.buffer, mysql.DB.username)
	Write1lenmsg(mysql.buffer, mysql.prepare_password(seed, seed2))
	WriteNullTerminatedString(mysql.buffer, mysql.DB.database)
	WriteNullTerminatedString(mysql.buffer, "mysql_native_password")
	msg := make([]byte, mysql.buffer.Len())
	copy(msg, mysql.buffer.Bytes())
	mysql.writemsg(msg)
	mysql.buffer.Reset()
	_, _, _, _, err := mysql.readmsg()
	if err == nil {
		mysql.Status = true
	}

}
func (mysql *Mysql_Conn) readmsg() (rowsAffected, lastInsertId int64, result int, errmsg string, err error) {
	msglen, err := mysql.readOneMsg()
	if err != nil {
		return
	}

	buffer := mysql.buffer.Bytes()[:msglen]

	switch buffer[0] {
	case 0: //ok报文
		var r, l int

		mysql.buffer.Next(1)
		r, err = ReadLength_Coded_Binary(mysql.buffer)
		if err != nil {
			return
		}
		l, err = ReadLength_Coded_Binary(mysql.buffer)
		if err != nil {
			return
		}
		mysql.buffer.Shift(4)
		//mysql.serverStatus = binary.LittleEndian.Uint16(mysql.buffer.Next(2))
		if err != nil {
			return
		}
		return int64(r), int64(l), 0, "", nil
	case 255: //err报文
		mysql.buffer.Next(1)
		errcode := binary.LittleEndian.Uint16(mysql.buffer.Next(2))
		mysql.buffer.Shift(6)
		msg, err := ioutil.ReadAll(mysql.buffer)
		if err != nil {
			return 0, 0, 255, "", err
		}
		return 0, 0, 255, strconv.Itoa(int(errcode)) + "-" + string(msg), nil
	case 254: //EOF报文
		mysql.buffer.Shift(5)
		return 0, 0, 254, "", nil
	default: //Result Set报文
		result, err = ReadLength_Coded_Binary(mysql.buffer)
		return
	}
	return 0, 0, 0, "", nil
}

//至少读一条消息
func (mysql *Mysql_Conn) readOneMsg() (msglen int, err error) {
	//DEBUG(mysql.buffer.Bytes())
	for mysql.buffer.Len() < 3 { //至少包含长度
		err = mysql.read()
		if err != nil {
			return
		}
	}
	//DEBUG(mysql.buffer.Bytes())
	b := mysql.buffer.Next(3)
	msglen = int(uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16)
	if msglen > max_packet_size {
		return 0, errors.New("EOF")
	}
	for mysql.buffer.Len() < msglen+1 { //至少包含一条消息的长度
		err = mysql.read()
		if err != nil {
			return 0, err
		}
	}
	mysql.msg_no = mysql.buffer.Next(1)[0]
	return
}
func (mysql *Mysql_Conn) writemsg(msg []byte) error {
	msglen := len(msg)
	if msglen > max_packet_size {
		return errors.New("消息大于最大长度" + strconv.Itoa(max_packet_size))
	}
	mysql.buffer.Reset()
	b := mysql.buffer.Make(3)
	b[0] = byte(msglen)
	b[1] = byte(msglen >> 8)
	b[2] = byte(msglen >> 16)
	mysql.msg_no++
	mysql.buffer.WriteByte(mysql.msg_no)
	mysql.buffer.Write(msg)

	_, err := mysql.conn.Write(mysql.buffer.Bytes())
	if err != nil {
		mysql.Status = false
		mysql.Close()
		return err
	}

	return err
}
func (mysql *Mysql_Conn) prepare_password(seed, seed2 []byte) []byte {
	if mysql.DB.passwd == "" {
		return []byte{}
	}
	h := sha1.New()
	h.Write(Str2bytes(mysql.DB.passwd))
	s1 := h.Sum(nil)
	h.Reset()
	h.Write(s1)
	s2 := h.Sum(nil)
	h.Reset()
	h.Write(seed)
	h.Write(seed2)
	h.Write(s2)
	s3 := h.Sum(nil)
	reply := make([]byte, len(s1))
	for k, _ := range s1 {
		reply[k] = s1[k] ^ s3[k]
	}
	return reply

}
func Write1lenmsg(write *MsgBuffer, msg []byte) {
	msglen := len(msg)
	if msglen > 255 {
		return
	}
	write.WriteByte(uint8(msglen))
	write.Write(msg)
}

func WriteNullTerminatedString(write *MsgBuffer, msg string) {
	write.Write(Str2bytes(msg))
	write.WriteByte(0)
}
func ReadLength_Coded_Binary(buf *MsgBuffer) (int, error) {
	if buf.Len() == 0 {
		return 0, errors.New("ReadLength_Coded_Binary err: buff length 0")
	}
	lentype, _ := buf.ReadByte()
	switch {
	case lentype < 251:
		return int(lentype), nil
	case lentype == 251:
		return 0, errors.New("NULL")
	case lentype == 252:
		if buf.Len() < 2 {
			return 0, errors.New("ReadLength_Coded_Binary err1")
		}
		return int(binary.LittleEndian.Uint16(buf.Next(2))), nil
	case lentype == 253:
		if buf.Len() < 3 {
			return 0, errors.New("ReadLength_Coded_Binary err2")
		}
		b := buf.Next(3)
		return int(uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16), nil
	case lentype == 254:
		if buf.Len() < 8 {
			return 0, errors.New("ReadLength_Coded_Binary err3")
		}
		return int(binary.LittleEndian.Uint64(buf.Next(8))), nil
	}
	return 0, nil
}
func ReadNullTerminatedString(msg *MsgBuffer) (string, error) {
	var b []byte
	top := msg.Bytes()
	for k, v := range top {
		if v == 0 {
			b = make([]byte, k)
			copy(b, msg.Next(k+1))
			break
		}
	}
	return Bytes2str(b), nil
}

func ReadLength_Coded_Byte(msg *MsgBuffer) ([]byte, error) {
	msglen, err := ReadLength_Coded_Binary(msg)
	if err != nil {
		if err.Error() == "NULL" {
			return []byte("NULL"), nil
		}
		return nil, err
	}
	if msglen == 0 {
		return nil, nil
	}
	return msg.Next(int(msglen)), nil
}
func ReadLengthCodedStringFromBuffer(msg *MsgBuffer, return_str bool) (string, error) {
	msglen, err := ReadLength_Coded_Binary(msg)
	if err != nil {
		if err.Error() == "NULL" {
			return "NULL", nil
		}
		return "", err
	}
	if msglen == 0 {
		return "", nil
	}
	if return_str {
		return string(msg.Next(int(msglen))), err
	}
	msg.Shift(int(msglen))
	return "", err
}
func Readint32frombin(msg *MsgBuffer) int32 {
	return int32(binary.LittleEndian.Uint32(msg.Next(4)))
}
func Readint64frombin(msg *MsgBuffer) int64 {
	return int64(binary.LittleEndian.Uint64(msg.Next(8)))
}
func Readuint64frombin(msg *MsgBuffer) uint64 {
	return binary.LittleEndian.Uint64(msg.Next(8))
}

var collations = map[string]byte{
	"big5_chinese_ci":          1,
	"latin2_czech_cs":          2,
	"dec8_swedish_ci":          3,
	"cp850_general_ci":         4,
	"latin1_german1_ci":        5,
	"hp8_english_ci":           6,
	"koi8r_general_ci":         7,
	"latin1_swedish_ci":        8,
	"latin2_general_ci":        9,
	"swe7_swedish_ci":          10,
	"ascii_general_ci":         11,
	"ujis_japanese_ci":         12,
	"sjis_japanese_ci":         13,
	"cp1251_bulgarian_ci":      14,
	"latin1_danish_ci":         15,
	"hebrew_general_ci":        16,
	"tis620_thai_ci":           18,
	"euckr_korean_ci":          19,
	"latin7_estonian_cs":       20,
	"latin2_hungarian_ci":      21,
	"koi8u_general_ci":         22,
	"cp1251_ukrainian_ci":      23,
	"gb2312_chinese_ci":        24,
	"gb2312":                   24,
	"greek_general_ci":         25,
	"cp1250_general_ci":        26,
	"latin2_croatian_ci":       27,
	"gbk_chinese_ci":           28,
	"gbk":                      28,
	"cp1257_lithuanian_ci":     29,
	"latin5_turkish_ci":        30,
	"latin1_german2_ci":        31,
	"armscii8_general_ci":      32,
	"utf8_general_ci":          33,
	"utf8":                     33,
	"cp1250_czech_cs":          34,
	"ucs2_general_ci":          35,
	"cp866_general_ci":         36,
	"keybcs2_general_ci":       37,
	"macce_general_ci":         38,
	"macroman_general_ci":      39,
	"cp852_general_ci":         40,
	"latin7_general_ci":        41,
	"latin7_general_cs":        42,
	"macce_bin":                43,
	"cp1250_croatian_ci":       44,
	"utf8mb4_general_ci":       45,
	"utf8mb4_bin":              46,
	"latin1_bin":               47,
	"latin1_general_ci":        48,
	"latin1_general_cs":        49,
	"cp1251_bin":               50,
	"cp1251_general_ci":        51,
	"cp1251_general_cs":        52,
	"macroman_bin":             53,
	"utf16_general_ci":         54,
	"utf16_bin":                55,
	"utf16le_general_ci":       56,
	"cp1256_general_ci":        57,
	"cp1257_bin":               58,
	"cp1257_general_ci":        59,
	"utf32_general_ci":         60,
	"utf32_bin":                61,
	"utf16le_bin":              62,
	"binary":                   63,
	"armscii8_bin":             64,
	"ascii_bin":                65,
	"cp1250_bin":               66,
	"cp1256_bin":               67,
	"cp866_bin":                68,
	"dec8_bin":                 69,
	"greek_bin":                70,
	"hebrew_bin":               71,
	"hp8_bin":                  72,
	"keybcs2_bin":              73,
	"koi8r_bin":                74,
	"koi8u_bin":                75,
	"latin2_bin":               77,
	"latin5_bin":               78,
	"latin7_bin":               79,
	"cp850_bin":                80,
	"cp852_bin":                81,
	"swe7_bin":                 82,
	"utf8_bin":                 83,
	"big5_bin":                 84,
	"euckr_bin":                85,
	"gb2312_bin":               86,
	"gbk_bin":                  87,
	"sjis_bin":                 88,
	"tis620_bin":               89,
	"ucs2_bin":                 90,
	"ujis_bin":                 91,
	"geostd8_general_ci":       92,
	"geostd8_bin":              93,
	"latin1_spanish_ci":        94,
	"cp932_japanese_ci":        95,
	"cp932_bin":                96,
	"eucjpms_japanese_ci":      97,
	"eucjpms_bin":              98,
	"cp1250_polish_ci":         99,
	"utf16_unicode_ci":         101,
	"utf16_icelandic_ci":       102,
	"utf16_latvian_ci":         103,
	"utf16_romanian_ci":        104,
	"utf16_slovenian_ci":       105,
	"utf16_polish_ci":          106,
	"utf16_estonian_ci":        107,
	"utf16_spanish_ci":         108,
	"utf16_swedish_ci":         109,
	"utf16_turkish_ci":         110,
	"utf16_czech_ci":           111,
	"utf16_danish_ci":          112,
	"utf16_lithuanian_ci":      113,
	"utf16_slovak_ci":          114,
	"utf16_spanish2_ci":        115,
	"utf16_roman_ci":           116,
	"utf16_persian_ci":         117,
	"utf16_esperanto_ci":       118,
	"utf16_hungarian_ci":       119,
	"utf16_sinhala_ci":         120,
	"utf16_german2_ci":         121,
	"utf16_croatian_ci":        122,
	"utf16_unicode_520_ci":     123,
	"utf16_vietnamese_ci":      124,
	"ucs2_unicode_ci":          128,
	"ucs2_icelandic_ci":        129,
	"ucs2_latvian_ci":          130,
	"ucs2_romanian_ci":         131,
	"ucs2_slovenian_ci":        132,
	"ucs2_polish_ci":           133,
	"ucs2_estonian_ci":         134,
	"ucs2_spanish_ci":          135,
	"ucs2_swedish_ci":          136,
	"ucs2_turkish_ci":          137,
	"ucs2_czech_ci":            138,
	"ucs2_danish_ci":           139,
	"ucs2_lithuanian_ci":       140,
	"ucs2_slovak_ci":           141,
	"ucs2_spanish2_ci":         142,
	"ucs2_roman_ci":            143,
	"ucs2_persian_ci":          144,
	"ucs2_esperanto_ci":        145,
	"ucs2_hungarian_ci":        146,
	"ucs2_sinhala_ci":          147,
	"ucs2_german2_ci":          148,
	"ucs2_croatian_ci":         149,
	"ucs2_unicode_520_ci":      150,
	"ucs2_vietnamese_ci":       151,
	"ucs2_general_mysql500_ci": 159,
	"utf32_unicode_ci":         160,
	"utf32_icelandic_ci":       161,
	"utf32_latvian_ci":         162,
	"utf32_romanian_ci":        163,
	"utf32_slovenian_ci":       164,
	"utf32_polish_ci":          165,
	"utf32_estonian_ci":        166,
	"utf32_spanish_ci":         167,
	"utf32_swedish_ci":         168,
	"utf32_turkish_ci":         169,
	"utf32_czech_ci":           170,
	"utf32_danish_ci":          171,
	"utf32_lithuanian_ci":      172,
	"utf32_slovak_ci":          173,
	"utf32_spanish2_ci":        174,
	"utf32_roman_ci":           175,
	"utf32_persian_ci":         176,
	"utf32_esperanto_ci":       177,
	"utf32_hungarian_ci":       178,
	"utf32_sinhala_ci":         179,
	"utf32_german2_ci":         180,
	"utf32_croatian_ci":        181,
	"utf32_unicode_520_ci":     182,
	"utf32_vietnamese_ci":      183,
	"utf8_unicode_ci":          192,
	"utf8_icelandic_ci":        193,
	"utf8_latvian_ci":          194,
	"utf8_romanian_ci":         195,
	"utf8_slovenian_ci":        196,
	"utf8_polish_ci":           197,
	"utf8_estonian_ci":         198,
	"utf8_spanish_ci":          199,
	"utf8_swedish_ci":          200,
	"utf8_turkish_ci":          201,
	"utf8_czech_ci":            202,
	"utf8_danish_ci":           203,
	"utf8_lithuanian_ci":       204,
	"utf8_slovak_ci":           205,
	"utf8_spanish2_ci":         206,
	"utf8_roman_ci":            207,
	"utf8_persian_ci":          208,
	"utf8_esperanto_ci":        209,
	"utf8_hungarian_ci":        210,
	"utf8_sinhala_ci":          211,
	"utf8_german2_ci":          212,
	"utf8_croatian_ci":         213,
	"utf8_unicode_520_ci":      214,
	"utf8_vietnamese_ci":       215,
	"utf8_general_mysql500_ci": 223,
	"utf8mb4_unicode_ci":       224,
	"utf8mb4_icelandic_ci":     225,
	"utf8mb4_latvian_ci":       226,
	"utf8mb4_romanian_ci":      227,
	"utf8mb4_slovenian_ci":     228,
	"utf8mb4_polish_ci":        229,
	"utf8mb4_estonian_ci":      230,
	"utf8mb4_spanish_ci":       231,
	"utf8mb4_swedish_ci":       232,
	"utf8mb4_turkish_ci":       233,
	"utf8mb4_czech_ci":         234,
	"utf8mb4_danish_ci":        235,
	"utf8mb4_lithuanian_ci":    236,
	"utf8mb4_slovak_ci":        237,
	"utf8mb4_spanish2_ci":      238,
	"utf8mb4_roman_ci":         239,
	"utf8mb4_persian_ci":       240,
	"utf8mb4_esperanto_ci":     241,
	"utf8mb4_hungarian_ci":     242,
	"utf8mb4_sinhala_ci":       243,
	"utf8mb4_german2_ci":       244,
	"utf8mb4_croatian_ci":      245,
	"utf8mb4_unicode_520_ci":   246,
	"utf8mb4_vietnamese_ci":    247,
}

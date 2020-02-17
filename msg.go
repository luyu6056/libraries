package libraries

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"

	//"encoding/json"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	. "protocol"
	"reflect"

	//"runtime"
	"strconv"
	"strings"
	"sync"
)

var aes_pool = &sync.Pool{
	New: func() interface{} {
		block, _ := aes.NewCipher(*aeskey1) //g_aeskey[:16]
		return &block
	},
}

var aeskey1 = &[]byte{106, 105, 110, 95, 116, 105, 97, 110, 95, 110, 105, 95, 99, 104, 105, 95}
var aeskey2 = &[]byte{108, 101, 95, 109, 101, 105, 95, 121, 111, 117, 63, 99, 104, 105, 95, 108}

func checkError(err error) {
	if err != nil {
		fmt.Println("Error: %s", err.Error())
		os.Exit(1)
	}
}

type Conn struct {
	Conn      net.Conn
	Msg_buf   []byte
	Buf       *bytes.Buffer
	Is_login  bool
	Player_ID []int64
	Rank_id   []int32
	Test      bool
	Seed      int64
	Pipi      string //皮皮的ID
	Guisi     string //鬼斯的ID
	Pika      string //皮卡丘的ID
	Item      map[int32]*Item
	Equip     map[int32][]*Item
	Level     int32
	send_lock sync.Mutex
}
type Item struct {
	Count int32
	Guid  string
}

//使用code去发送指定消息
func (this *Conn) Sendmsg(msgcode int32, data map[string]interface{}) {
	if _, ok := Protocol.Load(msgcode); !ok {
		panic("未定义的msgcode " + fmt.Sprint(msgcode))
	}
	this.send_lock.Lock()
	//根据协议构造请求参数
	b := get_byte_from_protocol(msgcode, data)

	if (len(b)+4)%16 > 0 {
		b = append(b, make([]byte, 16-(len(b)+4)%16)...)
	}
	handle := make([]byte, 4)
	binary.LittleEndian.PutUint32(handle, uint32(msgcode))
	b = append(b, handle...)
	msgLen := len(b)
	msg := make([]byte, 4+msgLen)
	binary.LittleEndian.PutUint32(msg, uint32(msgLen+4))

	copy(msg[4:], aesEncrypt(b))
	// 发送消息

	this.Conn.Write(msg)
	this.send_lock.Unlock()
	//fmt.Println("发送消息", msgCodeToType[msgcode], data)
}

//接收消息
func (this *Conn) Readmsg() (result interface{}) {
	var err error
	var msgCode int32
	if _, err = io.ReadFull(this.Conn, this.Msg_buf); err != nil {
		//fmt.Println("失败")
		return nil
	}
	// parse len
	msglen := binary.LittleEndian.Uint32(this.Msg_buf)
	msglen -= 4
	//fmt.Println(msglen)
	if msglen < 0 {
		return nil
	}
	if msglen > 10240 {
		fmt.Println("包体过长", msglen)
	}
	this.Buf.Reset()
	this.Buf.Write(make([]byte, msglen))
	if _, err = io.ReadFull(this.Conn, this.Buf.Bytes()); err != nil {
		return nil
	}
	//解密
	msgData := aesDecrypt(this.Buf)
	msgCode = int32(binary.LittleEndian.Uint32(msgData[len(msgData)-4:]))

	if _, ok := Protocol.Load(msgCode); !ok {
		panic("收到未设定msgcode" + strconv.FormatInt(int64(msgCode), 32))
	}
	return this.U2LS_handle(msgData, msgCode)
}

func get_byte_from_protocol(msgcode int32, data map[string]interface{}) []byte {
	//特殊代码

	if msgcode == 6056 {
		for key, _ := range data {
			Protocol.Store(msgcode, [][]string{[]string{key, key}})
		}

	}
	var b []byte
	if v, ok := Protocol.Load(msgcode); ok {
		list := v.([][]string)
		for _, arr := range list {
			//var s []string
			var tmp_b []byte
			switch arr[0] {
			case "int8":
				tmp_b = make([]byte, 1)
				i, _ := strconv.ParseInt(fmt.Sprint(data[arr[1]]), 10, 8)
				tmp_b[0] = byte(i)
			case "int16":
				tmp_b = make([]byte, 2)
				i, _ := strconv.ParseInt(fmt.Sprint(data[arr[1]]), 10, 16)
				binary.LittleEndian.PutUint16(tmp_b, uint16(i))
			case "int64":
				tmp_b = make([]byte, 8)
				i, _ := strconv.ParseInt(fmt.Sprint(data[arr[1]]), 10, 64)
				binary.LittleEndian.PutUint64(tmp_b, uint64(i))
			case "int32":
				tmp_b = make([]byte, 4)
				i, _ := strconv.ParseInt(fmt.Sprint(data[arr[1]]), 10, 32)
				binary.LittleEndian.PutUint32(tmp_b, uint32(i))
			case "string":
				b := []byte(data[arr[1]].(string))
				tmp_b = make([]byte, len(b)+2)
				binary.LittleEndian.PutUint16(tmp_b, uint16(len(b)))
				copy(tmp_b[2:], b)
			default:
				if msgTypeToCode[arr[0]] != 0 && data[arr[1]] != nil {
					tmp_b = get_byte_from_protocol(msgTypeToCode[arr[0]], data[arr[1]].(map[string]interface{}))
				} else {
					s, _ := Preg_match_result("vector\\<([^>]+)>", arr[0], -1)
					if len(s) != 0 {

						length := 0
						ref := reflect.ValueOf(data[arr[1]])
						if ref.Kind() == reflect.Slice {
							length = ref.Len()
						}
						tmp_b = make([]byte, 2)
						binary.LittleEndian.PutUint16(tmp_b, uint16(length))
						for i := 0; i < length; i++ {
							if field := ref.Index(i).Interface(); field != nil {
								if msgTypeToCode[s[0][1]] > 0 {
									tmp_b = append(tmp_b, get_byte_from_protocol(msgTypeToCode[s[0][1]], field.(map[string]interface{}))...)
								} else {
									tmp_b = append(tmp_b, get_byte_from_protocol(6056, map[string]interface{}{s[0][1]: field})...)
								}
							}

						}

					} else {
						fmt.Println("msg.go无法处理", arr[0])
					}
				}

			}
			b = append(b, tmp_b...)

		}
	}

	return b
}

var Protocol sync.Map //map[int32][][]string
var msgCodeToType map[int32]string
var msgTypeToCode map[string]int32

func Init_cfg(file string) {
	msgCodeToType = make(map[int32]string)
	msgTypeToCode = make(map[string]int32)
	//解析lua里面的配置文件
	data, err := ioutil.ReadFile(file)
	if err != nil {
		panic("配置文件读取错误")
	}
	for _, config := range strings.Split(string(data), "|") {
		lines := strings.Split(config, "?")
		headArr := strings.Split(lines[0], " ")
		if len(headArr) < 2 {
			continue
		}
		code_name := headArr[0]
		code, err := strconv.ParseInt(headArr[1], 10, 32)
		if err != nil {
			panic("配置文件code解析错误")
		}
		arr := make([][]string, len(lines)-1)
		for i := 0; i < len(lines)-1; i++ {
			arr[i] = strings.Split(lines[i+1], " ")
		}
		Protocol.Store(int32(code), arr)
		msgCodeToType[int32(code)] = code_name
		msgTypeToCode[code_name] = int32(code)
	}
}

func aesEncrypt(origData []byte) []byte {
	if len(origData)%16 > 0 {
		fmt.Println("AesEncrypt error:", len(origData), "%16 != 0")
		return origData
	}
	block := aes_pool.Get().(*cipher.Block)
	blockMode := cipher.NewCBCEncrypter(*block, *aeskey2) // g_aeskey[16:]
	crypted := make([]byte, len(origData))
	blockMode.CryptBlocks(crypted, origData)
	aes_pool.Put(block)
	return crypted
}

func aesDecrypt(buf *bytes.Buffer) []byte {
	origData := make([]byte, buf.Len())
	if buf.Len()%16 > 0 {
		copy(origData, buf.Bytes())
		fmt.Println("AesDecrypt error:", buf.Len(), "%16 != 0")
		return origData
	}
	block := aes_pool.Get().(*cipher.Block)
	blockMode := cipher.NewCBCDecrypter(*block, *aeskey2) // g_aeskey[16:]
	blockMode.CryptBlocks(origData, buf.Bytes())
	aes_pool.Put(block)
	return origData
}

func (this *Conn) U2LS_handle(bin []byte, cmd int32) interface{} {
	switch cmd {
	case CMD_MSG_LS2U_ResponseEnterServerResult:

		data, _ := READ_MSG_LS2U_ResponseEnterServerResult(bin)
		return data
	case CMD_MSG_GS2U_Login:
		data, _ := READ_MSG_GS2U_Login(bin)
		return data
	case CMD_MSG_GS2U_Get_OutdoorInfo_Result:
		this.Is_login = true
		data, _ := READ_MSG_GS2U_Get_OutdoorInfo_Result(bin)
		return data
	case CMD_MSG_GS2U_Get_Arena_Players_Ret:
		data, _ := READ_MSG_GS2U_Get_Arena_Players_Ret(bin)
		this.Player_ID = []int64{}
		this.Rank_id = nil
		for _, v := range data.ArenaPlayers {
			this.Player_ID = append(this.Player_ID, v.Player_id)
			this.Rank_id = append(this.Rank_id, v.Rank)
		}
		this.Test = false
		return data
	case CMD_MSG_GS2U_WorldBoss_Dare:
		this.Test = false
		data, _ := READ_MSG_GS2U_WorldBoss_Dare(bin)

		return data
	case CMD_MSG_GS2U_Legion_IntoDare_Result:
		data, _ := READ_MSG_GS2U_Legion_IntoDare_Result(bin)
		this.Test = false
		return data
	case CMD_MSG_GS2U_Legion_Dare_ReportServer_Result:
		data, _ := READ_MSG_GS2U_Legion_Dare_ReportServer_Result(bin)
		this.Test = false
		return data
	case CMD_MSG_GS2U_CampaignBoss_Dare_Result:
		data, _ := READ_MSG_GS2U_CampaignBoss_Dare_Result(bin)
		fmt.Println(data)
		return data
	case CMD_MSG_GS2U_Begin_Arena_Challenge_FightResult:
		data, _ := READ_MSG_GS2U_Begin_Arena_Challenge_FightResult(bin)
		this.Test = false
		return data
	case CMD_MSG_GS2U_HeroList:
		data, _ := READ_MSG_GS2U_HeroList(bin)
		for _, v := range data.List {
			if v.ConfigId == 20020822 && this.Pipi == "" {
				this.Pipi = strconv.Itoa(int(v.Guid))
			}
			if v.ConfigId == 20002001 && this.Pika == "" {
				this.Pika = strconv.Itoa(int(v.Guid))
			}
			if v.ConfigId == 20011206 && this.Guisi == "" {
				this.Guisi = strconv.Itoa(int(v.Guid))
			}
		}
		return data
	case CMD_MSG_GS2U_Hero:
		data, _ := READ_MSG_GS2U_Hero(bin)
		if data.ConfigId == 20011206 && this.Guisi == "" {
			this.Guisi = strconv.Itoa(int(data.Guid))
		}
		return data
	case CMD_MSG_GS2U_ItemList:
		data, _ := READ_MSG_GS2U_ItemList(bin)
		for _, v := range data.List {
			this.Item[v.ConfigId] = &Item{Count: v.Count, Guid: strconv.Itoa(int(v.Guid))}
		}
		return data
	case CMD_MSG_GS2U_Item:
		data, _ := READ_MSG_GS2U_Item(bin)
		this.Item[data.ConfigId] = &Item{Count: data.Count, Guid: strconv.Itoa(int(data.Guid))}
		return data
	case CMD_MSG_GS2U_EquipList:
		data, _ := READ_MSG_GS2U_EquipList(bin)
		for _, v := range data.List {
			g := strconv.Itoa(int(v.Guid))
			if _, ok := this.Equip[v.ConfigId]; !ok {
				this.Equip[v.ConfigId] = []*Item{&Item{Guid: g, Count: int32(v.Level)}}
			} else {
				var isexist bool
				for _, e := range this.Equip[v.ConfigId] {
					if e.Guid == g {
						isexist = true
					}
				}
				if !isexist {
					this.Equip[v.ConfigId] = append(this.Equip[v.ConfigId], &Item{Guid: g, Count: int32(v.Level)})
				}
			}
		}
		return data
	case CMD_MSG_GS2U_Equip:
		data, _ := READ_MSG_GS2U_Equip(bin)
		g := strconv.Itoa(int(data.Guid))
		if _, ok := this.Equip[data.ConfigId]; !ok {
			this.Equip[data.ConfigId] = []*Item{&Item{Guid: g, Count: int32(data.Level)}}
		} else {
			var isexist bool
			for _, e := range this.Equip[data.ConfigId] {
				if e.Guid == g {
					isexist = true
				}
			}
			if !isexist {
				this.Equip[data.ConfigId] = append(this.Equip[data.ConfigId], &Item{Guid: g, Count: int32(data.Level)})
			}
		}
		return data
	case CMD_MSG_GS2U_Player_Attribute_Modify:
		data, _ := READ_MSG_GS2U_Player_Attribute_Modify(bin)
		switch data.Key {
		case 0:
			this.Level = data.Value
		}
		return data
	}
	return nil
	switch cmd {
	case CMD_MSG_LS2U_ResponseEnterServerResult:
		data, _ := READ_MSG_LS2U_ResponseEnterServerResult(bin)
		return data
	case CMD_MSG_GameAreaInfo:
		data, _ := READ_MSG_GameAreaInfo(bin)
		return data
	case CMD_MSG_GameServerInfo:
		data, _ := READ_MSG_GameServerInfo(bin)
		return data
	case CMD_MSG_U2LS_RandAccount:
		data, _ := READ_MSG_U2LS_RandAccount(bin)
		return data
	case CMD_MSG_LS2U_RandAccount:
		data, _ := READ_MSG_LS2U_RandAccount(bin)
		return data
	case CMD_MSG_U2LS_Login:
		data, _ := READ_MSG_U2LS_Login(bin)
		return data
	case CMD_MSG_LS2U_LoginResult:
		data, _ := READ_MSG_LS2U_LoginResult(bin)
		return data
	case CMD_MSG_U2LS_GetServerList:
		data, _ := READ_MSG_U2LS_GetServerList(bin)
		return data
	case CMD_MSG_U2LS_GetServerList_For_SDK:
		data, _ := READ_MSG_U2LS_GetServerList_For_SDK(bin)
		return data
	case CMD_MSG_LS2U_SendServerList:
		data, _ := READ_MSG_LS2U_SendServerList(bin)
		return data
	case CMD_MSG_U2LS_RequestEnterServer:
		data, _ := READ_MSG_U2LS_RequestEnterServer(bin)
		return data

	case CMD_MSG_GS2U_HeartBeat:
		data, _ := READ_MSG_GS2U_HeartBeat(bin)
		return data

	case CMD_MSG_Announcement:
		data, _ := READ_MSG_Announcement(bin)
		return data
	case CMD_MSG_U2LS_QueryAnnouncement:
		data, _ := READ_MSG_U2LS_QueryAnnouncement(bin)
		return data
	case CMD_MSG_LS2U_AnnouncementList:
		data, _ := READ_MSG_LS2U_AnnouncementList(bin)
		return data
	case CMD_MSG_U2LS_QueryGameTips:
		data, _ := READ_MSG_U2LS_QueryGameTips(bin)
		return data
	case CMD_MSG_LS2U_QueryGameTipsList:
		data, _ := READ_MSG_LS2U_QueryGameTipsList(bin)
		return data

	case CMD_MSG_GS2U_CreatePlayer_Result:
		data, _ := READ_MSG_GS2U_CreatePlayer_Result(bin)
		return data

	case CMD_MSG_GS2U_PlayerInfo:
		data, _ := READ_MSG_GS2U_PlayerInfo(bin)

		return data

	case CMD_MSG_GS2U_HeroValhalla:
		data, _ := READ_MSG_GS2U_HeroValhalla(bin)

		return data
	case CMD_MSG_GS2U_HeroValhalla_List:
		data, _ := READ_MSG_GS2U_HeroValhalla_List(bin)

		return data
	case CMD_MSG_GS2U_Rune:
		data, _ := READ_MSG_GS2U_Rune(bin)

		return data
	case CMD_MSG_GS2U_Rune_List:
		data, _ := READ_MSG_GS2U_Rune_List(bin)

		return data
	case CMD_MSG_GS2U_Pet:
		data, _ := READ_MSG_GS2U_Pet(bin)

		return data
	case CMD_MSG_GS2U_Pet_List:
		data, _ := READ_MSG_GS2U_Pet_List(bin)

		return data

	case CMD_MSG_GS2U_GlyphsList:
		data, _ := READ_MSG_GS2U_GlyphsList(bin)

		return data
	case CMD_MSG_GS2U_Glyphs_Chip_List:
		data, _ := READ_MSG_GS2U_Glyphs_Chip_List(bin)

		return data

	case CMD_MSG_GS2U_Flush:
		data, _ := READ_MSG_GS2U_Flush(bin)

		return data
	case CMD_MSG_GS2U_Player_Value:
		data, _ := READ_MSG_GS2U_Player_Value(bin)

		return data
	case CMD_MSG_GS2U_World_Value:
		data, _ := READ_MSG_GS2U_World_Value(bin)

		return data
	case CMD_MSG_GS2U_Varibale_List:
		data, _ := READ_MSG_GS2U_Varibale_List(bin)

		return data
	case CMD_MSG_U2GS_Buy_Value:
		data, _ := READ_MSG_U2GS_Buy_Value(bin)

		return data
	case CMD_MSG_GS2U_Buy_Value_Result:
		data, _ := READ_MSG_GS2U_Buy_Value_Result(bin)

		return data
	case CMD_MSG_U2GS_VIPShop_Buy:
		data, _ := READ_MSG_U2GS_VIPShop_Buy(bin)

		return data
	case CMD_MSG_GS2U_VIPShop_Buy_Result:
		data, _ := READ_MSG_GS2U_VIPShop_Buy_Result(bin)

		return data
	case CMD_MSG_U2GS_Player_Walk:
		data, _ := READ_MSG_U2GS_Player_Walk(bin)

		return data
	case CMD_MSG_GS2U_Player_Walk_Pro:
		data, _ := READ_MSG_GS2U_Player_Walk_Pro(bin)

		return data
	case CMD_MSG_GS2U_Player_Walk_Pro_New:
		data, _ := READ_MSG_GS2U_Player_Walk_Pro_New(bin)

		return data
	case CMD_MSG_GS2U_Player_Walk_Info_Change:
		data, _ := READ_MSG_GS2U_Player_Walk_Info_Change(bin)

		return data
	case CMD_MSG_GS2U_Player_Walk_Del:
		data, _ := READ_MSG_GS2U_Player_Walk_Del(bin)

		return data
	case CMD_MSG_GS2U_Clothes_List:
		data, _ := READ_MSG_GS2U_Clothes_List(bin)

		return data
	case CMD_MSG_U2GS_Clothes_Buy:
		data, _ := READ_MSG_U2GS_Clothes_Buy(bin)

		return data
	case CMD_MSG_GS2U_Clothes_Buy_Ret:
		data, _ := READ_MSG_GS2U_Clothes_Buy_Ret(bin)

		return data
	case CMD_MSG_U2GS_Clothes_Select:
		data, _ := READ_MSG_U2GS_Clothes_Select(bin)

		return data
	case CMD_MSG_GS2U_Clothes_Select_Ret:
		data, _ := READ_MSG_GS2U_Clothes_Select_Ret(bin)

		return data
	case CMD_MSG_GS2U_Title:
		data, _ := READ_MSG_GS2U_Title(bin)

		return data
	case CMD_MSG_GS2U_Title_List:
		data, _ := READ_MSG_GS2U_Title_List(bin)

		return data
	case CMD_MSG_U2GS_Title_Select:
		data, _ := READ_MSG_U2GS_Title_Select(bin)

		return data
	case CMD_MSG_GS2U_Title_Select_Ret:
		data, _ := READ_MSG_GS2U_Title_Select_Ret(bin)

		return data
	case CMD_MSG_U2GS_Title_DelRedPoint:
		data, _ := READ_MSG_U2GS_Title_DelRedPoint(bin)

		return data
	case CMD_MSG_Dare_Equip:
		data, _ := READ_MSG_Dare_Equip(bin)

		return data
	case CMD_MSG_Dare_Rune:
		data, _ := READ_MSG_Dare_Rune(bin)

		return data
	case CMD_MSG_Dare_Pet:
		data, _ := READ_MSG_Dare_Pet(bin)

		return data
	case CMD_MSG_Dare_Hero_Data:
		data, _ := READ_MSG_Dare_Hero_Data(bin)

		return data
	case CMD_MSG_IntoDareTeamInfo:
		data, _ := READ_MSG_IntoDareTeamInfo(bin)

		return data
	case CMD_MSG_Dare_Hero_Data_2_Svr:
		data, _ := READ_MSG_Dare_Hero_Data_2_Svr(bin)

		return data
	case CMD_MSG_FightOperator:
		data, _ := READ_MSG_FightOperator(bin)

		return data
	case CMD_MSG_FightOperatorList:
		data, _ := READ_MSG_FightOperatorList(bin)

		return data
	case CMD_MSG_U2GS_Convoy_Double_Open:
		data, _ := READ_MSG_U2GS_Convoy_Double_Open(bin)

		return data
	case CMD_MSG_GS2U_Convoy_Double_Open:
		data, _ := READ_MSG_GS2U_Convoy_Double_Open(bin)

		return data
	case CMD_MSG_GS2U_Convoy_Info:
		data, _ := READ_MSG_GS2U_Convoy_Info(bin)

		return data
	case CMD_MSG_U2GS_Convoy_BuyLootNum:
		data, _ := READ_MSG_U2GS_Convoy_BuyLootNum(bin)

		return data
	case CMD_MSG_GS2U_Convoy_BuyLootNum:
		data, _ := READ_MSG_GS2U_Convoy_BuyLootNum(bin)

		return data
	case CMD_MSG_U2GS_Convoy_Get_PlayerInfo:
		data, _ := READ_MSG_U2GS_Convoy_Get_PlayerInfo(bin)

		return data
	case CMD_MSG_GS2U_Convoy_Get_PlayerInfo:
		data, _ := READ_MSG_GS2U_Convoy_Get_PlayerInfo(bin)

		return data
	case CMD_MSG_U2GS_Convoy_Get_HelpList:
		data, _ := READ_MSG_U2GS_Convoy_Get_HelpList(bin)

		return data
	case CMD_MSG_GS2U_Convoy_Get_HelpList:
		data, _ := READ_MSG_GS2U_Convoy_Get_HelpList(bin)

		return data
	case CMD_MSG_U2GS_Convoy_Request_Help:
		data, _ := READ_MSG_U2GS_Convoy_Request_Help(bin)

		return data
	case CMD_MSG_GS2U_Convoy_Request_Help:
		data, _ := READ_MSG_GS2U_Convoy_Request_Help(bin)

		return data
	case CMD_MSG_GS2U_Convoy_Answer_Help:
		data, _ := READ_MSG_GS2U_Convoy_Answer_Help(bin)

		return data
	case CMD_MSG_GS2U_Convoy_Timeout_Help_Other:
		data, _ := READ_MSG_GS2U_Convoy_Timeout_Help_Other(bin)

		return data
	case CMD_MSG_GS2U_Convoy_Request_Help_Other:
		data, _ := READ_MSG_GS2U_Convoy_Request_Help_Other(bin)

		return data
	case CMD_MSG_U2GS_Convoy_Answer_Help_Other:
		data, _ := READ_MSG_U2GS_Convoy_Answer_Help_Other(bin)

		return data
	case CMD_MSG_GS2U_Convoy_Answer_Help_Other:
		data, _ := READ_MSG_GS2U_Convoy_Answer_Help_Other(bin)

		return data
	case CMD_MSG_U2GS_Convoy_Refresh_Star:
		data, _ := READ_MSG_U2GS_Convoy_Refresh_Star(bin)

		return data
	case CMD_MSG_GS2U_Convoy_Refresh_Star:
		data, _ := READ_MSG_GS2U_Convoy_Refresh_Star(bin)

		return data
	case CMD_MSG_U2GS_Convoy_Start:
		data, _ := READ_MSG_U2GS_Convoy_Start(bin)

		return data
	case CMD_MSG_GS2U_Convoy_Start:
		data, _ := READ_MSG_GS2U_Convoy_Start(bin)

		return data
	case CMD_MSG_U2GS_Convoy_IntoScene:
		data, _ := READ_MSG_U2GS_Convoy_IntoScene(bin)

		return data
	case CMD_MSG_GS2U_Convoy_ScenePlayer:
		data, _ := READ_MSG_GS2U_Convoy_ScenePlayer(bin)

		return data
	case CMD_MSG_GS2U_Convoy_IntoScene:
		data, _ := READ_MSG_GS2U_Convoy_IntoScene(bin)

		return data
	case CMD_MSG_U2GS_Convoy_OutScene:
		data, _ := READ_MSG_U2GS_Convoy_OutScene(bin)

		return data
	case CMD_MSG_U2GS_Convoy_ScenePlayer_Ex:
		data, _ := READ_MSG_U2GS_Convoy_ScenePlayer_Ex(bin)

		return data
	case CMD_MSG_GS2U_Convoy_ScenePlayer_Ex:
		data, _ := READ_MSG_GS2U_Convoy_ScenePlayer_Ex(bin)

		return data
	case CMD_MSG_U2GS_Convoy_Loot:
		data, _ := READ_MSG_U2GS_Convoy_Loot(bin)

		return data
	case CMD_MSG_GS2U_Convoy_Loot:
		data, _ := READ_MSG_GS2U_Convoy_Loot(bin)

		return data
	case CMD_MSG_GS2U_Convoy_Loot_FightResult:
		data, _ := READ_MSG_GS2U_Convoy_Loot_FightResult(bin)

		return data
	case CMD_MSG_GS2U_Convoy_Loot_Report:
		data, _ := READ_MSG_GS2U_Convoy_Loot_Report(bin)

		return data
	case CMD_MSG_GS2U_Convoy_BeLoot:
		data, _ := READ_MSG_GS2U_Convoy_BeLoot(bin)

		return data
	case CMD_MSG_U2GS_Convoy_Loot_Log:
		data, _ := READ_MSG_U2GS_Convoy_Loot_Log(bin)

		return data
	case CMD_MSG_convoy_loot_log:
		data, _ := READ_MSG_convoy_loot_log(bin)

		return data
	case CMD_MSG_GS2U_Convoy_Loot_Log:
		data, _ := READ_MSG_GS2U_Convoy_Loot_Log(bin)

		return data
	case CMD_MSG_U2GS_War_Fail_Log_Save:
		data, _ := READ_MSG_U2GS_War_Fail_Log_Save(bin)

		return data
	case CMD_MSG_GS2U_MainScene_Info:
		data, _ := READ_MSG_GS2U_MainScene_Info(bin)

		return data
	case CMD_MSG_U2GS_Change_Team:
		data, _ := READ_MSG_U2GS_Change_Team(bin)

		return data
	case CMD_MSG_GS2U_Change_Team_Result:
		data, _ := READ_MSG_GS2U_Change_Team_Result(bin)

		return data
	case CMD_MSG_U2GS_Save_BattleArray:
		data, _ := READ_MSG_U2GS_Save_BattleArray(bin)

		return data
	case CMD_MSG_GS2U_Save_BattleArray_Result:
		data, _ := READ_MSG_GS2U_Save_BattleArray_Result(bin)

		return data
	case CMD_MSG_U2GS_Help_Team_Pos_Buy:
		data, _ := READ_MSG_U2GS_Help_Team_Pos_Buy(bin)

		return data
	case CMD_MSG_GS2U_Help_Team_Pos_Buy:
		data, _ := READ_MSG_GS2U_Help_Team_Pos_Buy(bin)

		return data
	case CMD_MSG_U2GS_Change_PlayerName:
		data, _ := READ_MSG_U2GS_Change_PlayerName(bin)

		return data
	case CMD_MSG_GS2U_Change_PlayerName_Result:
		data, _ := READ_MSG_GS2U_Change_PlayerName_Result(bin)

		return data
	case CMD_MSG_U2GS_Change_PlayerIcon:
		data, _ := READ_MSG_U2GS_Change_PlayerIcon(bin)

		return data
	case CMD_MSG_GS2U_Change_PlayerIcon_Result:
		data, _ := READ_MSG_GS2U_Change_PlayerIcon_Result(bin)

		return data
	case CMD_MSG_U2GS_Select_Camp:
		data, _ := READ_MSG_U2GS_Select_Camp(bin)

		return data
	case CMD_MSG_GS2U_Select_Camp_Result:
		data, _ := READ_MSG_GS2U_Select_Camp_Result(bin)

		return data
	case CMD_MSG_GS2U_PlayerIconFrame_List:
		data, _ := READ_MSG_GS2U_PlayerIconFrame_List(bin)

		return data
	case CMD_MSG_U2GS_PlayerIconFrame_Buy:
		data, _ := READ_MSG_U2GS_PlayerIconFrame_Buy(bin)

		return data
	case CMD_MSG_GS2U_PlayerIconFrame_Buy_Result:
		data, _ := READ_MSG_GS2U_PlayerIconFrame_Buy_Result(bin)

		return data
	case CMD_MSG_U2GS_QueMailList:
		data, _ := READ_MSG_U2GS_QueMailList(bin)

		return data
	case CMD_MSG_GS2U_MailInfo:
		data, _ := READ_MSG_GS2U_MailInfo(bin)

		return data
	case CMD_MSG_GS2U_MailList:
		data, _ := READ_MSG_GS2U_MailList(bin)

		return data
	case CMD_MSG_U2GS_ReadMail:
		data, _ := READ_MSG_U2GS_ReadMail(bin)

		return data
	case CMD_MSG_U2GS_GetMailItem:
		data, _ := READ_MSG_U2GS_GetMailItem(bin)

		return data
	case CMD_MSG_GS2U_GetMailItemResult:
		data, _ := READ_MSG_GS2U_GetMailItemResult(bin)

		return data
	case CMD_MSG_U2GS_DeleteMail:
		data, _ := READ_MSG_U2GS_DeleteMail(bin)

		return data
	case CMD_MSG_GS2U_DeleteMail:
		data, _ := READ_MSG_GS2U_DeleteMail(bin)

		return data
	case CMD_MSG_U2GS_SendMail:
		data, _ := READ_MSG_U2GS_SendMail(bin)

		return data
	case CMD_MSG_GS2U_SendMailResult:
		data, _ := READ_MSG_GS2U_SendMailResult(bin)

		return data
	case CMD_MSG_U2GS_UseItem:
		data, _ := READ_MSG_U2GS_UseItem(bin)

		return data
	case CMD_MSG_GS2U_UseItemResult:
		data, _ := READ_MSG_GS2U_UseItemResult(bin)

		return data
	case CMD_MSG_AttentionPoint:
		data, _ := READ_MSG_AttentionPoint(bin)

		return data
	case CMD_MSG_GS2U_AttentionPointList:
		data, _ := READ_MSG_GS2U_AttentionPointList(bin)

		return data
	case CMD_MSG_GS2U_Notice:
		data, _ := READ_MSG_GS2U_Notice(bin)

		return data
	case CMD_MSG_GS2U_MessageBox:
		data, _ := READ_MSG_GS2U_MessageBox(bin)

		return data
	case CMD_MSG_GS2U_ShortActivityInfo:
		data, _ := READ_MSG_GS2U_ShortActivityInfo(bin)

		return data
	case CMD_MSG_GS2U_ResponseShortActivityList:
		data, _ := READ_MSG_GS2U_ResponseShortActivityList(bin)

		return data
	case CMD_MSG_GS2U_RemoveActivity:
		data, _ := READ_MSG_GS2U_RemoveActivity(bin)

		return data
	case CMD_MSG_ActivityAwardItemInfo:
		data, _ := READ_MSG_ActivityAwardItemInfo(bin)

		return data
	case CMD_MSG_ActivityAwardInfo:
		data, _ := READ_MSG_ActivityAwardInfo(bin)

		return data
	case CMD_MSG_GS2U_Activity_OS_Data:
		data, _ := READ_MSG_GS2U_Activity_OS_Data(bin)

		return data
	case CMD_MSG_Activity_OS_Buy:
		data, _ := READ_MSG_Activity_OS_Buy(bin)

		return data
	case CMD_MSG_GS2U_Activity_OS_Info:
		data, _ := READ_MSG_GS2U_Activity_OS_Info(bin)

		return data
	case CMD_MSG_U2GS_Activity_OS_Buy:
		data, _ := READ_MSG_U2GS_Activity_OS_Buy(bin)

		return data
	case CMD_MSG_GS2U_Activity_OS_Buy_Result:
		data, _ := READ_MSG_GS2U_Activity_OS_Buy_Result(bin)

		return data
	case CMD_MSG_U2GS_Activity_OS_GetReward:
		data, _ := READ_MSG_U2GS_Activity_OS_GetReward(bin)

		return data
	case CMD_MSG_GS2U_Activity_OS_GetReward_Result:
		data, _ := READ_MSG_GS2U_Activity_OS_GetReward_Result(bin)

		return data
	case CMD_MSG_GS2U_Activity_OS2_Data:
		data, _ := READ_MSG_GS2U_Activity_OS2_Data(bin)

		return data
	case CMD_MSG_GS2U_Activity_OS2_Info:
		data, _ := READ_MSG_GS2U_Activity_OS2_Info(bin)

		return data
	case CMD_MSG_U2GS_Activity_OS2_GetReward:
		data, _ := READ_MSG_U2GS_Activity_OS2_GetReward(bin)

		return data
	case CMD_MSG_GS2U_Activity_OS2_GetReward_Result:
		data, _ := READ_MSG_GS2U_Activity_OS2_GetReward_Result(bin)

		return data
	case CMD_MSG_GS2U_Long_Sign_List:
		data, _ := READ_MSG_GS2U_Long_Sign_List(bin)

		return data
	case CMD_MSG_U2GS_Long_Sign_GetReward:
		data, _ := READ_MSG_U2GS_Long_Sign_GetReward(bin)

		return data
	case CMD_MSG_GS2U_Long_Sign_GetReward_Result:
		data, _ := READ_MSG_GS2U_Long_Sign_GetReward_Result(bin)

		return data
	case CMD_MSG_GS2U_Activity_RedPackage:
		data, _ := READ_MSG_GS2U_Activity_RedPackage(bin)

		return data
	case CMD_MSG_GS2U_Activity_RedPackage_Info:
		data, _ := READ_MSG_GS2U_Activity_RedPackage_Info(bin)

		return data
	case CMD_MSG_U2GS_Activity_RedPackage_Get:
		data, _ := READ_MSG_U2GS_Activity_RedPackage_Get(bin)

		return data
	case CMD_MSG_GS2U_Activity_RedPackage_Get_Result:
		data, _ := READ_MSG_GS2U_Activity_RedPackage_Get_Result(bin)

		return data
	case CMD_MSG_Lucky_Cat_Activity_Count:
		data, _ := READ_MSG_Lucky_Cat_Activity_Count(bin)

		return data
	case CMD_MSG_GS2U_Lucky_Cat_Activity:
		data, _ := READ_MSG_GS2U_Lucky_Cat_Activity(bin)

		return data
	case CMD_MSG_U2GS_Lucky_Cat_Get:
		data, _ := READ_MSG_U2GS_Lucky_Cat_Get(bin)

		return data
	case CMD_MSG_GS2U_Lucky_Cat_Get:
		data, _ := READ_MSG_GS2U_Lucky_Cat_Get(bin)

		return data
	case CMD_MSG_GS2U_7DayHapply_Activity:
		data, _ := READ_MSG_GS2U_7DayHapply_Activity(bin)

		return data
	case CMD_MSG_LevelUpCostInfo:
		data, _ := READ_MSG_LevelUpCostInfo(bin)

		return data
	case CMD_MSG_U2GS_Hero_Level_Up:
		data, _ := READ_MSG_U2GS_Hero_Level_Up(bin)

		return data
	case CMD_MSG_GS2U_Hero_Level_Up_Result:
		data, _ := READ_MSG_GS2U_Hero_Level_Up_Result(bin)

		return data
	case CMD_MSG_GS2U_Del_Hero:
		data, _ := READ_MSG_GS2U_Del_Hero(bin)

		return data
	case CMD_MSG_U2GS_Hero_Quality_Up:
		data, _ := READ_MSG_U2GS_Hero_Quality_Up(bin)

		return data
	case CMD_MSG_GS2U_Hero_Quality_Up_Result:
		data, _ := READ_MSG_GS2U_Hero_Quality_Up_Result(bin)

		return data
	case CMD_MSG_U2GS_Hero_AwakeUp:
		data, _ := READ_MSG_U2GS_Hero_AwakeUp(bin)

		return data
	case CMD_MSG_GS2U_Hero_AwardUp_Result:
		data, _ := READ_MSG_GS2U_Hero_AwardUp_Result(bin)

		return data
	case CMD_MSG_U2GS_Hero_Talent_Break:
		data, _ := READ_MSG_U2GS_Hero_Talent_Break(bin)

		return data
	case CMD_MSG_GS2U_Hero_Talent_Break_Result:
		data, _ := READ_MSG_GS2U_Hero_Talent_Break_Result(bin)

		return data
	case CMD_MSG_Item_Cost:
		data, _ := READ_MSG_Item_Cost(bin)

		return data
	case CMD_MSG_U2GS_Hero_Skill_LevelUp:
		data, _ := READ_MSG_U2GS_Hero_Skill_LevelUp(bin)

		return data
	case CMD_MSG_GS2U_Hero_Skill_LevelUp_Result:
		data, _ := READ_MSG_GS2U_Hero_Skill_LevelUp_Result(bin)

		return data
	case CMD_MSG_U2GS_HeroGlyphs_Select:
		data, _ := READ_MSG_U2GS_HeroGlyphs_Select(bin)

		return data
	case CMD_MSG_GS2U_HeroGlyphs_Select_Result:
		data, _ := READ_MSG_GS2U_HeroGlyphs_Select_Result(bin)

		return data
	case CMD_MSG_U2GS_HeroGlyphs_Activation:
		data, _ := READ_MSG_U2GS_HeroGlyphs_Activation(bin)

		return data
	case CMD_MSG_GS2U_HeroGlyphs_Activation_Result:
		data, _ := READ_MSG_GS2U_HeroGlyphs_Activation_Result(bin)

		return data
	case CMD_MSG_U2GS_HeroGlyphs_UpLevel:
		data, _ := READ_MSG_U2GS_HeroGlyphs_UpLevel(bin)

		return data
	case CMD_MSG_GS2U_HeroGlyphs_UpLevel_Result:
		data, _ := READ_MSG_GS2U_HeroGlyphs_UpLevel_Result(bin)

		return data
	case CMD_MSG_U2GS_HeroValhalla_UpLevel:
		data, _ := READ_MSG_U2GS_HeroValhalla_UpLevel(bin)

		return data
	case CMD_MSG_GS2U_HeroValhalla_UpLevel_Result:
		data, _ := READ_MSG_GS2U_HeroValhalla_UpLevel_Result(bin)

		return data
	case CMD_MSG_U2GS_HeroValhalla_UpStep:
		data, _ := READ_MSG_U2GS_HeroValhalla_UpStep(bin)

		return data
	case CMD_MSG_GS2U_HeroValhalla_UpStep_Result:
		data, _ := READ_MSG_GS2U_HeroValhalla_UpStep_Result(bin)

		return data
	case CMD_MSG_GS2U_Del_Equip:
		data, _ := READ_MSG_GS2U_Del_Equip(bin)

		return data
	case CMD_MSG_U2GS_Hero_Equip_On:
		data, _ := READ_MSG_U2GS_Hero_Equip_On(bin)

		return data
	case CMD_MSG_GS2U_Hero_Equip_On_Result:
		data, _ := READ_MSG_GS2U_Hero_Equip_On_Result(bin)

		return data
	case CMD_MSG_U2GS_Hero_Equip_Off:
		data, _ := READ_MSG_U2GS_Hero_Equip_Off(bin)

		return data
	case CMD_MSG_GS2U_Hero_Equip_Off_Result:
		data, _ := READ_MSG_GS2U_Hero_Equip_Off_Result(bin)

		return data
	case CMD_MSG_U2GS_Hero_Equip_OneKey_On:
		data, _ := READ_MSG_U2GS_Hero_Equip_OneKey_On(bin)

		return data
	case CMD_MSG_GS2U_Hero_Equip_OneKey_On_Result:
		data, _ := READ_MSG_GS2U_Hero_Equip_OneKey_On_Result(bin)

		return data
	case CMD_MSG_U2GS_Hero_Equip_OneKey_Off:
		data, _ := READ_MSG_U2GS_Hero_Equip_OneKey_Off(bin)

		return data
	case CMD_MSG_GS2U_Hero_Equip_OneKey_Off_Result:
		data, _ := READ_MSG_GS2U_Hero_Equip_OneKey_Off_Result(bin)

		return data
	case CMD_MSG_U2GS_Equip_Level_Up:
		data, _ := READ_MSG_U2GS_Equip_Level_Up(bin)

		return data
	case CMD_MSG_GS2U_Equip_Level_Up_Result:
		data, _ := READ_MSG_GS2U_Equip_Level_Up_Result(bin)

		return data
	case CMD_MSG_Equip_OneKey_Info:
		data, _ := READ_MSG_Equip_OneKey_Info(bin)

		return data
	case CMD_MSG_U2GS_OneKey_LevelUp:
		data, _ := READ_MSG_U2GS_OneKey_LevelUp(bin)

		return data
	case CMD_MSG_GS2U_OneKey_LevelUp_Result:
		data, _ := READ_MSG_GS2U_OneKey_LevelUp_Result(bin)

		return data
	case CMD_MSG_U2GS_Equip_Quality_Up:
		data, _ := READ_MSG_U2GS_Equip_Quality_Up(bin)

		return data
	case CMD_MSG_GS2U_Equip_Quality_Up_Result:
		data, _ := READ_MSG_GS2U_Equip_Quality_Up_Result(bin)

		return data
	case CMD_MSG_U2GS_Equip_AwakeUp:
		data, _ := READ_MSG_U2GS_Equip_AwakeUp(bin)

		return data
	case CMD_MSG_GS2U_Equip_AwardUp_Result:
		data, _ := READ_MSG_GS2U_Equip_AwardUp_Result(bin)

		return data
	case CMD_MSG_U2GS_Equip_Refine:
		data, _ := READ_MSG_U2GS_Equip_Refine(bin)

		return data
	case CMD_MSG_GS2U_Equip_Refine_Result:
		data, _ := READ_MSG_GS2U_Equip_Refine_Result(bin)

		return data
	case CMD_MSG_U2GS_Equip_Enchant_Refresh:
		data, _ := READ_MSG_U2GS_Equip_Enchant_Refresh(bin)

		return data
	case CMD_MSG_GS2U_Equip_Enchant_Refresh_Result:
		data, _ := READ_MSG_GS2U_Equip_Enchant_Refresh_Result(bin)

		return data
	case CMD_MSG_U2GS_Equip_Enchant_Replace:
		data, _ := READ_MSG_U2GS_Equip_Enchant_Replace(bin)

		return data
	case CMD_MSG_GS2U_Equip_Enchant_Replace:
		data, _ := READ_MSG_GS2U_Equip_Enchant_Replace(bin)

		return data
	case CMD_MSG_U2GS_GiveUp_RefreshOutList:
		data, _ := READ_MSG_U2GS_GiveUp_RefreshOutList(bin)

		return data
	case CMD_MSG_GS2U_GiveUp_RefreshOutList_Result:
		data, _ := READ_MSG_GS2U_GiveUp_RefreshOutList_Result(bin)

		return data
	case CMD_MSG_U2GS_Substitution:
		data, _ := READ_MSG_U2GS_Substitution(bin)

		return data
	case CMD_MSG_GS2U_Substitution_Ret:
		data, _ := READ_MSG_GS2U_Substitution_Ret(bin)

		return data
	case CMD_MSG_decompose_info:
		data, _ := READ_MSG_decompose_info(bin)

		return data
	case CMD_MSG_U2GS_Hero_And_Equip_Decompose_Predict:
		data, _ := READ_MSG_U2GS_Hero_And_Equip_Decompose_Predict(bin)

		return data
	case CMD_MSG_GS2U_Hero_And_Equip_Decompose_Predict_Result:
		data, _ := READ_MSG_GS2U_Hero_And_Equip_Decompose_Predict_Result(bin)

		return data
	case CMD_MSG_U2GS_Hero_And_Equip_Decompose:
		data, _ := READ_MSG_U2GS_Hero_And_Equip_Decompose(bin)

		return data
	case CMD_MSG_GS2U_Hero_And_Equip_Decompose_Result:
		data, _ := READ_MSG_GS2U_Hero_And_Equip_Decompose_Result(bin)

		return data
	case CMD_MSG_U2GS_Reset_Hero_Equip_Predict:
		data, _ := READ_MSG_U2GS_Reset_Hero_Equip_Predict(bin)

		return data
	case CMD_MSG_GS2U_Reset_Hero_Equip_Predict_Result:
		data, _ := READ_MSG_GS2U_Reset_Hero_Equip_Predict_Result(bin)

		return data
	case CMD_MSG_U2GS_Reset_Hero_Equip:
		data, _ := READ_MSG_U2GS_Reset_Hero_Equip(bin)

		return data
	case CMD_MSG_GS2U_Reset_Hero_Equip_Result:
		data, _ := READ_MSG_GS2U_Reset_Hero_Equip_Result(bin)

		return data
	case CMD_MSG_U2GS_Hero_Compose:
		data, _ := READ_MSG_U2GS_Hero_Compose(bin)

		return data
	case CMD_MSG_GS2U_Hero_Compose_Result:
		data, _ := READ_MSG_GS2U_Hero_Compose_Result(bin)

		return data
	case CMD_MSG_U2GS_Get_Skill_Point:
		data, _ := READ_MSG_U2GS_Get_Skill_Point(bin)

		return data
	case CMD_MSG_U2GS_Buy_SkillPoint:
		data, _ := READ_MSG_U2GS_Buy_SkillPoint(bin)

		return data
	case CMD_MSG_GS2U_Buy_SkillPoint_Result:
		data, _ := READ_MSG_GS2U_Buy_SkillPoint_Result(bin)

		return data
	case CMD_MSG_U2GS_Get_Hero_Inherit_Cost:
		data, _ := READ_MSG_U2GS_Get_Hero_Inherit_Cost(bin)

		return data
	case CMD_MSG_GS2U_Get_Hero_Inherit_Cost_Result:
		data, _ := READ_MSG_GS2U_Get_Hero_Inherit_Cost_Result(bin)

		return data
	case CMD_MSG_U2GS_Hero_Inherit:
		data, _ := READ_MSG_U2GS_Hero_Inherit(bin)

		return data
	case CMD_MSG_GS2U_Hero_Inherit_Result:
		data, _ := READ_MSG_GS2U_Hero_Inherit_Result(bin)

		return data
	case CMD_MSG_U2GS_NewGlyphs_Level_Up:
		data, _ := READ_MSG_U2GS_NewGlyphs_Level_Up(bin)

		return data
	case CMD_MSG_GS2U_NewGlyphs_Level_Up_Result:
		data, _ := READ_MSG_GS2U_NewGlyphs_Level_Up_Result(bin)

		return data
	case CMD_MSG_GS2U_Del_NewGlyphs:
		data, _ := READ_MSG_GS2U_Del_NewGlyphs(bin)

		return data
	case CMD_MSG_Glyphs_Change:
		data, _ := READ_MSG_Glyphs_Change(bin)

		return data
	case CMD_MSG_U2GS_Change_NewGlyphs:
		data, _ := READ_MSG_U2GS_Change_NewGlyphs(bin)

		return data
	case CMD_MSG_GS2U_Change_NewGlyphs:
		data, _ := READ_MSG_GS2U_Change_NewGlyphs(bin)

		return data
	case CMD_MSG_U2GS_NewGlyphs_Compose:
		data, _ := READ_MSG_U2GS_NewGlyphs_Compose(bin)

		return data
	case CMD_MSG_GS2U_NewGlyphs_Compose_Result:
		data, _ := READ_MSG_GS2U_NewGlyphs_Compose_Result(bin)

		return data
	case CMD_MSG_U2GS_Change_Chips:
		data, _ := READ_MSG_U2GS_Change_Chips(bin)

		return data
	case CMD_MSG_GS2U_Change_Chips_Result:
		data, _ := READ_MSG_GS2U_Change_Chips_Result(bin)

		return data
	case CMD_MSG_U2GS_Get_Can_Reborn_HeroList:
		data, _ := READ_MSG_U2GS_Get_Can_Reborn_HeroList(bin)

		return data
	case CMD_MSG_GS2U_Get_Can_Reborn_HeroList_Result:
		data, _ := READ_MSG_GS2U_Get_Can_Reborn_HeroList_Result(bin)

		return data
	case CMD_MSG_U2GS_Reborn_Hero:
		data, _ := READ_MSG_U2GS_Reborn_Hero(bin)

		return data
	case CMD_MSG_GS2U_Reborn_Hero_Result:
		data, _ := READ_MSG_GS2U_Reborn_Hero_Result(bin)

		return data
	case CMD_MSG_GS2U_Rune_Delete:
		data, _ := READ_MSG_GS2U_Rune_Delete(bin)

		return data
	case CMD_MSG_U2GS_Rune_PutOn:
		data, _ := READ_MSG_U2GS_Rune_PutOn(bin)

		return data
	case CMD_MSG_GS2U_Rune_PutOn_Result:
		data, _ := READ_MSG_GS2U_Rune_PutOn_Result(bin)

		return data
	case CMD_MSG_U2GS_Rune_PutOff:
		data, _ := READ_MSG_U2GS_Rune_PutOff(bin)

		return data
	case CMD_MSG_GS2U_Rune_PutOff_Result:
		data, _ := READ_MSG_GS2U_Rune_PutOff_Result(bin)

		return data
	case CMD_MSG_LevelUpCost:
		data, _ := READ_MSG_LevelUpCost(bin)

		return data
	case CMD_MSG_U2GS_Rune_UpLevel:
		data, _ := READ_MSG_U2GS_Rune_UpLevel(bin)

		return data
	case CMD_MSG_GS2U_Rune_UpLevel_Result:
		data, _ := READ_MSG_GS2U_Rune_UpLevel_Result(bin)

		return data
	case CMD_MSG_U2GS_Rune_Refine:
		data, _ := READ_MSG_U2GS_Rune_Refine(bin)

		return data
	case CMD_MSG_GS2U_Rune_Refine_Result:
		data, _ := READ_MSG_GS2U_Rune_Refine_Result(bin)

		return data
	case CMD_MSG_GS2U_RunesOut_Info:
		data, _ := READ_MSG_GS2U_RunesOut_Info(bin)

		return data
	case CMD_MSG_U2GS_RunesOut_Get:
		data, _ := READ_MSG_U2GS_RunesOut_Get(bin)

		return data
	case CMD_MSG_RunesOutItemInfo:
		data, _ := READ_MSG_RunesOutItemInfo(bin)

		return data
	case CMD_MSG_GS2U_RunesOut_Get:
		data, _ := READ_MSG_GS2U_RunesOut_Get(bin)

		return data
	case CMD_MSG_U2GS_RunesOut_PosSet:
		data, _ := READ_MSG_U2GS_RunesOut_PosSet(bin)

		return data
	case CMD_MSG_GS2U_RunesOut_PosSet:
		data, _ := READ_MSG_GS2U_RunesOut_PosSet(bin)

		return data
	case CMD_MSG_U2GS_RunesOut_log:
		data, _ := READ_MSG_U2GS_RunesOut_log(bin)

		return data
	case CMD_MSG_GS2U_RunesOut_log:
		data, _ := READ_MSG_GS2U_RunesOut_log(bin)

		return data
	case CMD_MSG_GS2U_Pet_Delete:
		data, _ := READ_MSG_GS2U_Pet_Delete(bin)

		return data
	case CMD_MSG_U2GS_Pet_GoOut:
		data, _ := READ_MSG_U2GS_Pet_GoOut(bin)

		return data
	case CMD_MSG_GS2U_Pet_GoOut:
		data, _ := READ_MSG_GS2U_Pet_GoOut(bin)

		return data
	case CMD_MSG_U2GS_Pet_PutOn:
		data, _ := READ_MSG_U2GS_Pet_PutOn(bin)

		return data
	case CMD_MSG_GS2U_Pet_PutOn_Result:
		data, _ := READ_MSG_GS2U_Pet_PutOn_Result(bin)

		return data
	case CMD_MSG_U2GS_Pet_PutOff:
		data, _ := READ_MSG_U2GS_Pet_PutOff(bin)

		return data
	case CMD_MSG_GS2U_Pet_PutOff_Result:
		data, _ := READ_MSG_GS2U_Pet_PutOff_Result(bin)

		return data
	case CMD_MSG_U2GS_Pet_UpLevel:
		data, _ := READ_MSG_U2GS_Pet_UpLevel(bin)

		return data
	case CMD_MSG_GS2U_Pet_UpLevel_Result:
		data, _ := READ_MSG_GS2U_Pet_UpLevel_Result(bin)

		return data
	case CMD_MSG_U2GS_Pet_UpStar:
		data, _ := READ_MSG_U2GS_Pet_UpStar(bin)

		return data
	case CMD_MSG_GS2U_Pet_UpStar_Result:
		data, _ := READ_MSG_GS2U_Pet_UpStar_Result(bin)

		return data
	case CMD_MSG_GS2U_CampaignInfo:
		data, _ := READ_MSG_GS2U_CampaignInfo(bin)

		return data
	case CMD_MSG_GS2U_CampaignChapterInfo:
		data, _ := READ_MSG_GS2U_CampaignChapterInfo(bin)

		return data
	case CMD_MSG_GS2U_Campaign_Chapter_List:
		data, _ := READ_MSG_GS2U_Campaign_Chapter_List(bin)

		return data
	case CMD_MSG_U2GS_Get_CampaignChapterAward:
		data, _ := READ_MSG_U2GS_Get_CampaignChapterAward(bin)

		return data
	case CMD_MSG_GS2U_Get_CampaignChapterAward_Result:
		data, _ := READ_MSG_GS2U_Get_CampaignChapterAward_Result(bin)

		return data
	case CMD_MSG_U2GS_Get_CampaignChapterAward_OneKey:
		data, _ := READ_MSG_U2GS_Get_CampaignChapterAward_OneKey(bin)

		return data
	case CMD_MSG_GS2U_Get_CampaignChapterAward_OneKey_Result:
		data, _ := READ_MSG_GS2U_Get_CampaignChapterAward_OneKey_Result(bin)

		return data
	case CMD_MSG_U2GS_ResetCampaign:
		data, _ := READ_MSG_U2GS_ResetCampaign(bin)

		return data
	case CMD_MSG_GS2U_ResertCampaign_Result:
		data, _ := READ_MSG_GS2U_ResertCampaign_Result(bin)

		return data
	case CMD_MSG_U2GS_IntoDareCampaign:
		data, _ := READ_MSG_U2GS_IntoDareCampaign(bin)

		return data
	case CMD_MSG_GS2U_IntoDareCampaign_Result:
		data, _ := READ_MSG_GS2U_IntoDareCampaign_Result(bin)

		return data
	case CMD_MSG_U2GS_DareCampaign_Reportserver:
		data, _ := READ_MSG_U2GS_DareCampaign_Reportserver(bin)

		return data
	case CMD_MSG_GS2U_DareCampaign_Reportserver_Result:
		data, _ := READ_MSG_GS2U_DareCampaign_Reportserver_Result(bin)

		return data
	case CMD_MSG_GS2U_DareCampaign_FirstDare_Award:
		data, _ := READ_MSG_GS2U_DareCampaign_FirstDare_Award(bin)

		return data
	case CMD_MSG_U2GS_SDCampaign:
		data, _ := READ_MSG_U2GS_SDCampaign(bin)

		return data
	case CMD_MSG_SDCampaign_Result:
		data, _ := READ_MSG_SDCampaign_Result(bin)

		return data
	case CMD_MSG_GS2U_SDCampaign_Result:
		data, _ := READ_MSG_GS2U_SDCampaign_Result(bin)

		return data
	case CMD_MSG_U2GS_CampaignBossList:
		data, _ := READ_MSG_U2GS_CampaignBossList(bin)

		return data
	case CMD_MSG_CampaignBoss:
		data, _ := READ_MSG_CampaignBoss(bin)

		return data
	case CMD_MSG_GS2U_CampaignBossList_Result:
		data, _ := READ_MSG_GS2U_CampaignBossList_Result(bin)

		return data
	case CMD_MSG_U2GS_CampaignBoss_Dare:
		data, _ := READ_MSG_U2GS_CampaignBoss_Dare(bin)

		return data
	case CMD_MSG_GS2U_CampaignBoss_Dare_Result:
		data, _ := READ_MSG_GS2U_CampaignBoss_Dare_Result(bin)

		return data
	case CMD_MSG_U2GS_CampaignBoss_Reportserver:
		data, _ := READ_MSG_U2GS_CampaignBoss_Reportserver(bin)

		return data
	case CMD_MSG_GS2U_CampaignBoss_Reportserver_Result:
		data, _ := READ_MSG_GS2U_CampaignBoss_Reportserver_Result(bin)

		return data
	case CMD_MSG_U2GS_CampaignBoss_Share:
		data, _ := READ_MSG_U2GS_CampaignBoss_Share(bin)

		return data
	case CMD_MSG_GS2U_CampaignBoss_Share_Result:
		data, _ := READ_MSG_GS2U_CampaignBoss_Share_Result(bin)

		return data
	case CMD_MSG_U2GS_CampaignBoss_GetReward:
		data, _ := READ_MSG_U2GS_CampaignBoss_GetReward(bin)

		return data
	case CMD_MSG_GS2U_CampaignBoss_GetReward_Result:
		data, _ := READ_MSG_GS2U_CampaignBoss_GetReward_Result(bin)

		return data
	case CMD_MSG_U2GS_CampaignBoss_AllHurtInfo:
		data, _ := READ_MSG_U2GS_CampaignBoss_AllHurtInfo(bin)

		return data
	case CMD_MSG_GS2U_CampaignBoss_AllHurtInfo:
		data, _ := READ_MSG_GS2U_CampaignBoss_AllHurtInfo(bin)

		return data
	case CMD_MSG_U2GS_CampaignBoss_AllHurtReward_Get:
		data, _ := READ_MSG_U2GS_CampaignBoss_AllHurtReward_Get(bin)

		return data
	case CMD_MSG_GS2U_CampaignBoss_AllHurtReward_Get:
		data, _ := READ_MSG_GS2U_CampaignBoss_AllHurtReward_Get(bin)

		return data
	case CMD_MSG_U2GS_CampaignResource_Info:
		data, _ := READ_MSG_U2GS_CampaignResource_Info(bin)

		return data
	case CMD_MSG_GS2U_CampaignResource:
		data, _ := READ_MSG_GS2U_CampaignResource(bin)

		return data
	case CMD_MSG_GS2U_CampaignResource_Info:
		data, _ := READ_MSG_GS2U_CampaignResource_Info(bin)

		return data
	case CMD_MSG_GS2U_CampaignResource_Info_Result:
		data, _ := READ_MSG_GS2U_CampaignResource_Info_Result(bin)

		return data
	case CMD_MSG_U2GS_CampaignResource_IntoDare:
		data, _ := READ_MSG_U2GS_CampaignResource_IntoDare(bin)

		return data
	case CMD_MSG_GS2U_CampaignResource_IntoDare_Result:
		data, _ := READ_MSG_GS2U_CampaignResource_IntoDare_Result(bin)

		return data
	case CMD_MSG_U2GS_CampaignResource_Reportserver:
		data, _ := READ_MSG_U2GS_CampaignResource_Reportserver(bin)

		return data
	case CMD_MSG_GS2U_CampaignResource_Reportserver_Result:
		data, _ := READ_MSG_GS2U_CampaignResource_Reportserver_Result(bin)

		return data
	case CMD_MSG_U2GS_CampaignResource_BuyCount:
		data, _ := READ_MSG_U2GS_CampaignResource_BuyCount(bin)

		return data
	case CMD_MSG_GS2U_CampaignResource_BuyCount_Result:
		data, _ := READ_MSG_GS2U_CampaignResource_BuyCount_Result(bin)

		return data
	case CMD_MSG_U2GS_CampaignResource_SD:
		data, _ := READ_MSG_U2GS_CampaignResource_SD(bin)

		return data
	case CMD_MSG_GS2U_CampaignResource_SD_Result:
		data, _ := READ_MSG_GS2U_CampaignResource_SD_Result(bin)

		return data
	case CMD_MSG_U2GS_CampaignResource_ClearCD:
		data, _ := READ_MSG_U2GS_CampaignResource_ClearCD(bin)

		return data
	case CMD_MSG_GS2U_CampaignResource_ClearCD_Result:
		data, _ := READ_MSG_GS2U_CampaignResource_ClearCD_Result(bin)

		return data
	case CMD_MSG_U2GS_Backtemple_Info:
		data, _ := READ_MSG_U2GS_Backtemple_Info(bin)

		return data
	case CMD_MSG_Backtemp_Hero:
		data, _ := READ_MSG_Backtemp_Hero(bin)

		return data
	case CMD_MSG_GS2U_Backtemple_Info:
		data, _ := READ_MSG_GS2U_Backtemple_Info(bin)

		return data
	case CMD_MSG_U2GS_Backtemple_Reset:
		data, _ := READ_MSG_U2GS_Backtemple_Reset(bin)

		return data
	case CMD_MSG_GS2U_Backtemple_Reset:
		data, _ := READ_MSG_GS2U_Backtemple_Reset(bin)

		return data
	case CMD_MSG_U2GS_Backtemple_GetBlood:
		data, _ := READ_MSG_U2GS_Backtemple_GetBlood(bin)

		return data
	case CMD_MSG_GS2U_Backtemple_GetBlood:
		data, _ := READ_MSG_GS2U_Backtemple_GetBlood(bin)

		return data
	case CMD_MSG_U2GS_Backtemple_UseBlood:
		data, _ := READ_MSG_U2GS_Backtemple_UseBlood(bin)

		return data
	case CMD_MSG_GS2U_Backtemple_UseBlood:
		data, _ := READ_MSG_GS2U_Backtemple_UseBlood(bin)

		return data
	case CMD_MSG_U2GS_Backtemple_Dare:
		data, _ := READ_MSG_U2GS_Backtemple_Dare(bin)

		return data
	case CMD_MSG_GS2U_Backtemple_Dare:
		data, _ := READ_MSG_GS2U_Backtemple_Dare(bin)

		return data
	case CMD_MSG_U2GS_Backtemple_Reportserver:
		data, _ := READ_MSG_U2GS_Backtemple_Reportserver(bin)

		return data
	case CMD_MSG_GS2U_Backtemple_Reportserver_Result:
		data, _ := READ_MSG_GS2U_Backtemple_Reportserver_Result(bin)

		return data
	case CMD_MSG_U2GS_Backtemple_GetBox:
		data, _ := READ_MSG_U2GS_Backtemple_GetBox(bin)

		return data
	case CMD_MSG_GS2U_Backtemple_GetBox:
		data, _ := READ_MSG_GS2U_Backtemple_GetBox(bin)

		return data
	case CMD_MSG_U2GS_TrainAmity_Info:
		data, _ := READ_MSG_U2GS_TrainAmity_Info(bin)

		return data
	case CMD_MSG_TrainCampaign:
		data, _ := READ_MSG_TrainCampaign(bin)

		return data
	case CMD_MSG_Train_Friend_Hero:
		data, _ := READ_MSG_Train_Friend_Hero(bin)

		return data
	case CMD_MSG_Train_Self_Hero:
		data, _ := READ_MSG_Train_Self_Hero(bin)

		return data
	case CMD_MSG_TrainAmity_Team:
		data, _ := READ_MSG_TrainAmity_Team(bin)

		return data
	case CMD_MSG_GS2U_TrainAmity_Info:
		data, _ := READ_MSG_GS2U_TrainAmity_Info(bin)

		return data
	case CMD_MSG_U2GS_TrainAmity_Flush_FriendHero:
		data, _ := READ_MSG_U2GS_TrainAmity_Flush_FriendHero(bin)

		return data
	case CMD_MSG_GS2U_TrainAmity_Flush_FriendHero:
		data, _ := READ_MSG_GS2U_TrainAmity_Flush_FriendHero(bin)

		return data
	case CMD_MSG_U2GS_TrainAmity_ChangeTeam:
		data, _ := READ_MSG_U2GS_TrainAmity_ChangeTeam(bin)

		return data
	case CMD_MSG_GS2U_TrainAmity_ChangeTeam:
		data, _ := READ_MSG_GS2U_TrainAmity_ChangeTeam(bin)

		return data
	case CMD_MSG_U2GS_TrainAmity_Dare:
		data, _ := READ_MSG_U2GS_TrainAmity_Dare(bin)

		return data
	case CMD_MSG_GS2U_TrainAmity_Dare:
		data, _ := READ_MSG_GS2U_TrainAmity_Dare(bin)

		return data
	case CMD_MSG_U2GS_TrainAmity_Reportserver:
		data, _ := READ_MSG_U2GS_TrainAmity_Reportserver(bin)

		return data
	case CMD_MSG_GS2U_TrainAmity_Reportserver_Result:
		data, _ := READ_MSG_GS2U_TrainAmity_Reportserver_Result(bin)

		return data
	case CMD_MSG_U2GS_TrainAmity_SD:
		data, _ := READ_MSG_U2GS_TrainAmity_SD(bin)

		return data
	case CMD_MSG_GS2U_TrainAmity_SD_Result:
		data, _ := READ_MSG_GS2U_TrainAmity_SD_Result(bin)

		return data
	case CMD_MSG_U2GS_TrainJob_Info:
		data, _ := READ_MSG_U2GS_TrainJob_Info(bin)

		return data
	case CMD_MSG_GS2U_TrainJob:
		data, _ := READ_MSG_GS2U_TrainJob(bin)

		return data
	case CMD_MSG_GS2U_TrainJob_Info:
		data, _ := READ_MSG_GS2U_TrainJob_Info(bin)

		return data
	case CMD_MSG_GS2U_TrainJob_Info_Result:
		data, _ := READ_MSG_GS2U_TrainJob_Info_Result(bin)

		return data
	case CMD_MSG_U2GS_TrainJob_Dare:
		data, _ := READ_MSG_U2GS_TrainJob_Dare(bin)

		return data
	case CMD_MSG_GS2U_TrainJob_Dare:
		data, _ := READ_MSG_GS2U_TrainJob_Dare(bin)

		return data
	case CMD_MSG_U2GS_TrainJob_Reportserver:
		data, _ := READ_MSG_U2GS_TrainJob_Reportserver(bin)

		return data
	case CMD_MSG_GS2U_TrainJob_Reportserver_Result:
		data, _ := READ_MSG_GS2U_TrainJob_Reportserver_Result(bin)

		return data
	case CMD_MSG_U2GS_TrainJob_SD:
		data, _ := READ_MSG_U2GS_TrainJob_SD(bin)

		return data
	case CMD_MSG_GS2U_TrainJob_SD_Result:
		data, _ := READ_MSG_GS2U_TrainJob_SD_Result(bin)

		return data
	case CMD_MSG_GS2U_SingleBoss_Card:
		data, _ := READ_MSG_GS2U_SingleBoss_Card(bin)

		return data
	case CMD_MSG_GS2U_SingleBoss_Open:
		data, _ := READ_MSG_GS2U_SingleBoss_Open(bin)

		return data
	case CMD_MSG_GS2U_SingleBoss:
		data, _ := READ_MSG_GS2U_SingleBoss(bin)

		return data
	case CMD_MSG_GS2U_SingleBoss_AllInfo:
		data, _ := READ_MSG_GS2U_SingleBoss_AllInfo(bin)

		return data
	case CMD_MSG_U2GS_SingleBoss_OpenCard:
		data, _ := READ_MSG_U2GS_SingleBoss_OpenCard(bin)

		return data
	case CMD_MSG_GS2U_SingleBoss_OpenCard:
		data, _ := READ_MSG_GS2U_SingleBoss_OpenCard(bin)

		return data
	case CMD_MSG_U2GS_SingleBoss_IntoDare:
		data, _ := READ_MSG_U2GS_SingleBoss_IntoDare(bin)

		return data
	case CMD_MSG_GS2U_SingleBoss_IntoDare_Result:
		data, _ := READ_MSG_GS2U_SingleBoss_IntoDare_Result(bin)

		return data
	case CMD_MSG_U2GS_SingleBoss_Reportserver:
		data, _ := READ_MSG_U2GS_SingleBoss_Reportserver(bin)

		return data
	case CMD_MSG_GS2U_SingleBoss_Reportserver_Result:
		data, _ := READ_MSG_GS2U_SingleBoss_Reportserver_Result(bin)

		return data
	case CMD_MSG_U2GS_SingleBoss_SD:
		data, _ := READ_MSG_U2GS_SingleBoss_SD(bin)

		return data
	case CMD_MSG_GS2U_SingleBoss_SD_Result:
		data, _ := READ_MSG_GS2U_SingleBoss_SD_Result(bin)

		return data
	case CMD_MSG_U2GS_Get_Diamond_Lottery_Info:
		data, _ := READ_MSG_U2GS_Get_Diamond_Lottery_Info(bin)

		return data
	case CMD_MSG_Gem_Bag_Info:
		data, _ := READ_MSG_Gem_Bag_Info(bin)

		return data
	case CMD_MSG_GS2U_Get_Diamond_Lottery_Info_Ret:
		data, _ := READ_MSG_GS2U_Get_Diamond_Lottery_Info_Ret(bin)

		return data
	case CMD_MSG_U2GS_Diamond_Lottery:
		data, _ := READ_MSG_U2GS_Diamond_Lottery(bin)

		return data
	case CMD_MSG_GS2U_Diamond_Lottery_Result:
		data, _ := READ_MSG_GS2U_Diamond_Lottery_Result(bin)

		return data
	case CMD_MSG_U2GS_Active_Lottery_Index:
		data, _ := READ_MSG_U2GS_Active_Lottery_Index(bin)

		return data
	case CMD_MSG_GS2U_Active_Lottery_Index_Result:
		data, _ := READ_MSG_GS2U_Active_Lottery_Index_Result(bin)

		return data
	case CMD_MSG_U2GS_OneKey_Get_Diamond:
		data, _ := READ_MSG_U2GS_OneKey_Get_Diamond(bin)

		return data
	case CMD_MSG_GS2U_OneKey_Get_Diamond_Result:
		data, _ := READ_MSG_GS2U_OneKey_Get_Diamond_Result(bin)

		return data
	case CMD_MSG_U2GS_OneKey_Diamond_Lottery:
		data, _ := READ_MSG_U2GS_OneKey_Diamond_Lottery(bin)

		return data
	case CMD_MSG_GS2U_OneKey_Diamond_Lottery_Result:
		data, _ := READ_MSG_GS2U_OneKey_Diamond_Lottery_Result(bin)

		return data
	case CMD_MSG_U2GS_Get_One_Diamond:
		data, _ := READ_MSG_U2GS_Get_One_Diamond(bin)

		return data
	case CMD_MSG_GS2U_Get_One_Diamond_Result:
		data, _ := READ_MSG_GS2U_Get_One_Diamond_Result(bin)

		return data
	case CMD_MSG_U2GS_Diamond_Hole_On:
		data, _ := READ_MSG_U2GS_Diamond_Hole_On(bin)

		return data
	case CMD_MSG_GS2U_Didmond_Hole_On_Result:
		data, _ := READ_MSG_GS2U_Didmond_Hole_On_Result(bin)

		return data
	case CMD_MSG_U2GS_Diamond_Hole_Off:
		data, _ := READ_MSG_U2GS_Diamond_Hole_Off(bin)

		return data
	case CMD_MSG_GS2U_Diamond_Hole_Off_Result:
		data, _ := READ_MSG_GS2U_Diamond_Hole_Off_Result(bin)

		return data
	case CMD_MSG_U2GS_Item_compound:
		data, _ := READ_MSG_U2GS_Item_compound(bin)

		return data
	case CMD_MSG_GS2U_Item_compound_Result:
		data, _ := READ_MSG_GS2U_Item_compound_Result(bin)

		return data
	case CMD_MSG_U2GS_Get_Arean_Players:
		data, _ := READ_MSG_U2GS_Get_Arean_Players(bin)

		return data
	case CMD_MSG_ArenaPlayer:
		data, _ := READ_MSG_ArenaPlayer(bin)

		return data
	case CMD_MSG_Top3Respect:
		data, _ := READ_MSG_Top3Respect(bin)

		return data

	case CMD_MSG_U2GS_Reset_CoolTime:
		data, _ := READ_MSG_U2GS_Reset_CoolTime(bin)

		return data
	case CMD_MSG_GS2U_Reset_CoolTime_Result:
		data, _ := READ_MSG_GS2U_Reset_CoolTime_Result(bin)

		return data
	case CMD_MSG_U2GS_Buy_ArenaNum:
		data, _ := READ_MSG_U2GS_Buy_ArenaNum(bin)

		return data
	case CMD_MSG_GS2U_Buy_ArenaNum_Result:
		data, _ := READ_MSG_GS2U_Buy_ArenaNum_Result(bin)

		return data
	case CMD_MSG_U2GS_Begin_Arena_Challenge:
		data, _ := READ_MSG_U2GS_Begin_Arena_Challenge(bin)

		return data
	case CMD_MSG_GS2U_Begin_Arena_Challenge_Fight:
		data, _ := READ_MSG_GS2U_Begin_Arena_Challenge_Fight(bin)

		return data

	case CMD_MSG_GS2U_Arena_Beat:
		data, _ := READ_MSG_GS2U_Arena_Beat(bin)

		return data
	case CMD_MSG_U2GS_Get_Arena_award:
		data, _ := READ_MSG_U2GS_Get_Arena_award(bin)

		return data
	case CMD_MSG_GS2U_Get_Arena_award_Result:
		data, _ := READ_MSG_GS2U_Get_Arena_award_Result(bin)

		return data
	case CMD_MSG_U2GS_Get_Battle_Record:
		data, _ := READ_MSG_U2GS_Get_Battle_Record(bin)

		return data
	case CMD_MSG_BattleRecord:
		data, _ := READ_MSG_BattleRecord(bin)

		return data
	case CMD_MSG_GS2U_Get_Battle_Record_Ret:
		data, _ := READ_MSG_GS2U_Get_Battle_Record_Ret(bin)

		return data
	case CMD_MSG_U2GS_Change_ArenaPlayers:
		data, _ := READ_MSG_U2GS_Change_ArenaPlayers(bin)

		return data
	case CMD_MSG_GS2U_Change_ArenaPlayers_Result:
		data, _ := READ_MSG_GS2U_Change_ArenaPlayers_Result(bin)

		return data
	case CMD_MSG_U2GS_Get_Goal_Award:
		data, _ := READ_MSG_U2GS_Get_Goal_Award(bin)

		return data
	case CMD_MSG_GS2U_Get_Goal_Award_Result:
		data, _ := READ_MSG_GS2U_Get_Goal_Award_Result(bin)

		return data
	case CMD_MSG_U2GS_Arena_Respect_Top3:
		data, _ := READ_MSG_U2GS_Arena_Respect_Top3(bin)

		return data
	case CMD_MSG_GS2U_Arena_Respect_Top3_Result:
		data, _ := READ_MSG_GS2U_Arena_Respect_Top3_Result(bin)

		return data
	case CMD_MSG_U2GS_Arena_Challenge_NTimes:
		data, _ := READ_MSG_U2GS_Arena_Challenge_NTimes(bin)

		return data
	case CMD_MSG_GS2U_Arena_Challenge_NTimes_Result:
		data, _ := READ_MSG_GS2U_Arena_Challenge_NTimes_Result(bin)

		return data
	case CMD_MSG_ShopItem:
		data, _ := READ_MSG_ShopItem(bin)

		return data
	case CMD_MSG_U2GS_QueryShop:
		data, _ := READ_MSG_U2GS_QueryShop(bin)

		return data
	case CMD_MSG_GS2U_ShopRequirement:
		data, _ := READ_MSG_GS2U_ShopRequirement(bin)

		return data
	case CMD_MSG_GS2U_ShopItemList:
		data, _ := READ_MSG_GS2U_ShopItemList(bin)

		return data
	case CMD_MSG_U2GS_BuyShopItem:
		data, _ := READ_MSG_U2GS_BuyShopItem(bin)

		return data
	case CMD_MSG_GS2U_BuyShopItemResult:
		data, _ := READ_MSG_GS2U_BuyShopItemResult(bin)

		return data
	case CMD_MSG_U2GS_RequestRefreshShop:
		data, _ := READ_MSG_U2GS_RequestRefreshShop(bin)

		return data
	case CMD_MSG_GS2U_ResponseRefreshShopResult:
		data, _ := READ_MSG_GS2U_ResponseRefreshShopResult(bin)

		return data
	case CMD_MSG_GS2U_LegionInfo:
		data, _ := READ_MSG_GS2U_LegionInfo(bin)

		return data
	case CMD_MSG_U2GS_Legion_Reset:
		data, _ := READ_MSG_U2GS_Legion_Reset(bin)

		return data
	case CMD_MSG_GS2U_Legion_Reset_Result:
		data, _ := READ_MSG_GS2U_Legion_Reset_Result(bin)

		return data
	case CMD_MSG_U2GS_Legion_IntoDare:
		data, _ := READ_MSG_U2GS_Legion_IntoDare(bin)

		return data

	case CMD_MSG_U2GS_Legion_Dare_ReportServer:
		data, _ := READ_MSG_U2GS_Legion_Dare_ReportServer(bin)

		return data
	case CMD_MSG_GS2U_Legion_Dare_ReportServer_Result:
		data, _ := READ_MSG_GS2U_Legion_Dare_ReportServer_Result(bin)

		return data
	case CMD_MSG_U2GS_Legion_SD:
		data, _ := READ_MSG_U2GS_Legion_SD(bin)

		return data
	case CMD_MSG_GS2U_Legion_SD_Result:
		data, _ := READ_MSG_GS2U_Legion_SD_Result(bin)

		return data
	case CMD_MSG_U2GS_Legion_JYCount_Buy:
		data, _ := READ_MSG_U2GS_Legion_JYCount_Buy(bin)

		return data
	case CMD_MSG_GS2U_Legion_JYCount_Buy_Result:
		data, _ := READ_MSG_GS2U_Legion_JYCount_Buy_Result(bin)

		return data
	case CMD_MSG_U2GS_Legion_Get_TargetAward:
		data, _ := READ_MSG_U2GS_Legion_Get_TargetAward(bin)

		return data
	case CMD_MSG_GS2U_Legion_Get_TargetAward_Result:
		data, _ := READ_MSG_GS2U_Legion_Get_TargetAward_Result(bin)

		return data
	case CMD_MSG_GS2U_Guild:
		data, _ := READ_MSG_GS2U_Guild(bin)

		return data
	case CMD_MSG_U2GS_Guild_List:
		data, _ := READ_MSG_U2GS_Guild_List(bin)

		return data
	case CMD_MSG_Guild_SimpleInfo:
		data, _ := READ_MSG_Guild_SimpleInfo(bin)

		return data
	case CMD_MSG_GS2U_Guild_List_Result:
		data, _ := READ_MSG_GS2U_Guild_List_Result(bin)

		return data
	case CMD_MSG_U2GS_Guild_Apply:
		data, _ := READ_MSG_U2GS_Guild_Apply(bin)

		return data
	case CMD_MSG_GS2U_Guild_Apply_Result:
		data, _ := READ_MSG_GS2U_Guild_Apply_Result(bin)

		return data
	case CMD_MSG_U2GS_Guild_Create:
		data, _ := READ_MSG_U2GS_Guild_Create(bin)

		return data
	case CMD_MSG_GS2U_Guild_Create_Result:
		data, _ := READ_MSG_GS2U_Guild_Create_Result(bin)

		return data
	case CMD_MSG_U2GS_Guild_DetailedInfo:
		data, _ := READ_MSG_U2GS_Guild_DetailedInfo(bin)

		return data
	case CMD_MSG_GS2U_Guild_Technology:
		data, _ := READ_MSG_GS2U_Guild_Technology(bin)

		return data
	case CMD_MSG_GS2U_Guild_DetailedInfo:
		data, _ := READ_MSG_GS2U_Guild_DetailedInfo(bin)

		return data
	case CMD_MSG_U2GS_Guild_Log:
		data, _ := READ_MSG_U2GS_Guild_Log(bin)

		return data
	case CMD_MSG_GS2U_Guild_Log:
		data, _ := READ_MSG_GS2U_Guild_Log(bin)

		return data
	case CMD_MSG_GS2U_Guild_Log_Result:
		data, _ := READ_MSG_GS2U_Guild_Log_Result(bin)

		return data
	case CMD_MSG_U2GS_Guild_Modify:
		data, _ := READ_MSG_U2GS_Guild_Modify(bin)

		return data
	case CMD_MSG_GS2U_Guild_Modify_Result:
		data, _ := READ_MSG_GS2U_Guild_Modify_Result(bin)

		return data
	case CMD_MSG_U2GS_Guild_PlayerList:
		data, _ := READ_MSG_U2GS_Guild_PlayerList(bin)

		return data
	case CMD_MSG_GS2U_Guild_Player:
		data, _ := READ_MSG_GS2U_Guild_Player(bin)

		return data
	case CMD_MSG_GS2U_Guild_PlayerList_Result:
		data, _ := READ_MSG_GS2U_Guild_PlayerList_Result(bin)

		return data
	case CMD_MSG_U2GS_Guild_ApplyList:
		data, _ := READ_MSG_U2GS_Guild_ApplyList(bin)

		return data
	case CMD_MSG_GS2U_Guild_Apply:
		data, _ := READ_MSG_GS2U_Guild_Apply(bin)

		return data
	case CMD_MSG_GS2U_Guild_ApplyList_Result:
		data, _ := READ_MSG_GS2U_Guild_ApplyList_Result(bin)

		return data
	case CMD_MSG_U2GS_Guild_Resuse:
		data, _ := READ_MSG_U2GS_Guild_Resuse(bin)

		return data
	case CMD_MSG_GS2U_Guild_Resuse_Result:
		data, _ := READ_MSG_GS2U_Guild_Resuse_Result(bin)

		return data
	case CMD_MSG_U2GS_Guild_Accept:
		data, _ := READ_MSG_U2GS_Guild_Accept(bin)

		return data
	case CMD_MSG_GS2U_Guild_Accept_Result:
		data, _ := READ_MSG_GS2U_Guild_Accept_Result(bin)

		return data
	case CMD_MSG_GS2U_Guild_Player_Delete:
		data, _ := READ_MSG_GS2U_Guild_Player_Delete(bin)

		return data
	case CMD_MSG_U2GS_Guild_SeePlayer:
		data, _ := READ_MSG_U2GS_Guild_SeePlayer(bin)

		return data
	case CMD_MSG_GS2U_Guild_SeePlayer_Result:
		data, _ := READ_MSG_GS2U_Guild_SeePlayer_Result(bin)

		return data
	case CMD_MSG_U2GS_Guild_PlayerJob:
		data, _ := READ_MSG_U2GS_Guild_PlayerJob(bin)

		return data
	case CMD_MSG_GS2U_Guild_PlayerJob_Result:
		data, _ := READ_MSG_GS2U_Guild_PlayerJob_Result(bin)

		return data
	case CMD_MSG_U2GS_Guild_Contribution:
		data, _ := READ_MSG_U2GS_Guild_Contribution(bin)

		return data
	case CMD_MSG_GS2U_Guild_Contribution_Result:
		data, _ := READ_MSG_GS2U_Guild_Contribution_Result(bin)

		return data
	case CMD_MSG_U2GS_Guild_SendMail:
		data, _ := READ_MSG_U2GS_Guild_SendMail(bin)

		return data
	case CMD_MSG_GS2U_Guild_SendMail_Result:
		data, _ := READ_MSG_GS2U_Guild_SendMail_Result(bin)

		return data
	case CMD_MSG_GS2U_Guild_Campaign_Chapter:
		data, _ := READ_MSG_GS2U_Guild_Campaign_Chapter(bin)

		return data
	case CMD_MSG_GS2U_Guild_Campaign_Info:
		data, _ := READ_MSG_GS2U_Guild_Campaign_Info(bin)

		return data
	case CMD_MSG_GS2U_Guild_Campaign_Reset:
		data, _ := READ_MSG_GS2U_Guild_Campaign_Reset(bin)

		return data
	case CMD_MSG_GS2U_Guild_Campaign_All:
		data, _ := READ_MSG_GS2U_Guild_Campaign_All(bin)

		return data
	case CMD_MSG_U2GS_Guild_Campaign_Dare:
		data, _ := READ_MSG_U2GS_Guild_Campaign_Dare(bin)

		return data
	case CMD_MSG_GS2U_Guild_Campaign_Dare_Result:
		data, _ := READ_MSG_GS2U_Guild_Campaign_Dare_Result(bin)

		return data
	case CMD_MSG_U2GS_Guild_Campaign_Reportserver:
		data, _ := READ_MSG_U2GS_Guild_Campaign_Reportserver(bin)

		return data
	case CMD_MSG_GS2U_Guild_Campaign_Reportserver_Result:
		data, _ := READ_MSG_GS2U_Guild_Campaign_Reportserver_Result(bin)

		return data
	case CMD_MSG_U2GS_Guild_Campaign_Hurt_Rank:
		data, _ := READ_MSG_U2GS_Guild_Campaign_Hurt_Rank(bin)

		return data
	case CMD_MSG_GS2U_Guild_Campaign_Hurt_Rank_Result:
		data, _ := READ_MSG_GS2U_Guild_Campaign_Hurt_Rank_Result(bin)

		return data
	case CMD_MSG_U2GS_Guild_Item:
		data, _ := READ_MSG_U2GS_Guild_Item(bin)

		return data
	case CMD_MSG_Guild_Item:
		data, _ := READ_MSG_Guild_Item(bin)

		return data
	case CMD_MSG_GS2U_Guild_Item_Result:
		data, _ := READ_MSG_GS2U_Guild_Item_Result(bin)

		return data
	case CMD_MSG_U2GS_Guild_Item_Apply:
		data, _ := READ_MSG_U2GS_Guild_Item_Apply(bin)

		return data
	case CMD_MSG_GS2U_Guild_Item_Apply_Result:
		data, _ := READ_MSG_GS2U_Guild_Item_Apply_Result(bin)

		return data
	case CMD_MSG_U2GS_Guild_Item_Apply_List:
		data, _ := READ_MSG_U2GS_Guild_Item_Apply_List(bin)

		return data
	case CMD_MSG_Guild_Item_Apply_Player:
		data, _ := READ_MSG_Guild_Item_Apply_Player(bin)

		return data
	case CMD_MSG_GS2U_Guild_Item_Apply_List_Result:
		data, _ := READ_MSG_GS2U_Guild_Item_Apply_List_Result(bin)

		return data
	case CMD_MSG_U2GS_Guild_Campaign_Reset:
		data, _ := READ_MSG_U2GS_Guild_Campaign_Reset(bin)

		return data
	case CMD_MSG_GS2U_Guild_Campaign_Reset_Result:
		data, _ := READ_MSG_GS2U_Guild_Campaign_Reset_Result(bin)

		return data
	case CMD_MSG_GS2U_Guild_Box_Award_Info:
		data, _ := READ_MSG_GS2U_Guild_Box_Award_Info(bin)

		return data
	case CMD_MSG_U2GS_Get_Guild_Box_Award:
		data, _ := READ_MSG_U2GS_Get_Guild_Box_Award(bin)

		return data
	case CMD_MSG_GS2U_Get_Guild_Box_Award_Result:
		data, _ := READ_MSG_GS2U_Get_Guild_Box_Award_Result(bin)

		return data
	case CMD_MSG_U2GS_Get_Player_GuildIllidan_Info:
		data, _ := READ_MSG_U2GS_Get_Player_GuildIllidan_Info(bin)

		return data
	case CMD_MSG_GS2U_Get_Player_GuildIllidan_Info_Result:
		data, _ := READ_MSG_GS2U_Get_Player_GuildIllidan_Info_Result(bin)

		return data
	case CMD_MSG_U2GS_GuildIllidan_Active_Once:
		data, _ := READ_MSG_U2GS_GuildIllidan_Active_Once(bin)

		return data
	case CMD_MSG_GS2U_GuildIllidan_Active_Once_Result:
		data, _ := READ_MSG_GS2U_GuildIllidan_Active_Once_Result(bin)

		return data
	case CMD_MSG_U2GS_Get_GuildIllidan_Award:
		data, _ := READ_MSG_U2GS_Get_GuildIllidan_Award(bin)

		return data
	case CMD_MSG_GS2U_Get_GuildIllidan_Award_Result:
		data, _ := READ_MSG_GS2U_Get_GuildIllidan_Award_Result(bin)

		return data
	case CMD_MSG_Rank_Player_Info:
		data, _ := READ_MSG_Rank_Player_Info(bin)

		return data
	case CMD_MSG_U2GS_Ranking_Level:
		data, _ := READ_MSG_U2GS_Ranking_Level(bin)

		return data
	case CMD_MSG_GS2U_Ranking_Level_Result:
		data, _ := READ_MSG_GS2U_Ranking_Level_Result(bin)

		return data
	case CMD_MSG_U2GS_Ranking_Fight:
		data, _ := READ_MSG_U2GS_Ranking_Fight(bin)

		return data
	case CMD_MSG_GS2U_Ranking_Fight_Result:
		data, _ := READ_MSG_GS2U_Ranking_Fight_Result(bin)

		return data
	case CMD_MSG_Rank_HeroFight_Info:
		data, _ := READ_MSG_Rank_HeroFight_Info(bin)

		return data
	case CMD_MSG_U2GS_Ranking_Career_HeroFight:
		data, _ := READ_MSG_U2GS_Ranking_Career_HeroFight(bin)

		return data
	case CMD_MSG_GS2U_Ranking_Career_HeroFight:
		data, _ := READ_MSG_GS2U_Ranking_Career_HeroFight(bin)

		return data
	case CMD_MSG_U2GS_Ranking_Campaign:
		data, _ := READ_MSG_U2GS_Ranking_Campaign(bin)

		return data
	case CMD_MSG_GS2U_Ranking_Campaign_Result:
		data, _ := READ_MSG_GS2U_Ranking_Campaign_Result(bin)

		return data
	case CMD_MSG_U2GS_Ranking_Legion:
		data, _ := READ_MSG_U2GS_Ranking_Legion(bin)

		return data
	case CMD_MSG_GS2U_Ranking_Legion_Result:
		data, _ := READ_MSG_GS2U_Ranking_Legion_Result(bin)

		return data
	case CMD_MSG_U2GS_Ranking_WorldBoss:
		data, _ := READ_MSG_U2GS_Ranking_WorldBoss(bin)

		return data
	case CMD_MSG_GS2U_Ranking_WorldBoss_Result:
		data, _ := READ_MSG_GS2U_Ranking_WorldBoss_Result(bin)

		return data
	case CMD_MSG_U2GS_Ranking_CampaignBoss:
		data, _ := READ_MSG_U2GS_Ranking_CampaignBoss(bin)

		return data
	case CMD_MSG_GS2U_Ranking_CampaignBoss_Result:
		data, _ := READ_MSG_GS2U_Ranking_CampaignBoss_Result(bin)

		return data
	case CMD_MSG_U2GS_Ranking_Arena:
		data, _ := READ_MSG_U2GS_Ranking_Arena(bin)

		return data
	case CMD_MSG_GS2U_Ranking_Arena_Result:
		data, _ := READ_MSG_GS2U_Ranking_Arena_Result(bin)

		return data
	case CMD_MSG_Rank_Guild_Info:
		data, _ := READ_MSG_Rank_Guild_Info(bin)

		return data
	case CMD_MSG_U2GS_Ranking_Guild:
		data, _ := READ_MSG_U2GS_Ranking_Guild(bin)

		return data
	case CMD_MSG_GS2U_Ranking_Guild_Result:
		data, _ := READ_MSG_GS2U_Ranking_Guild_Result(bin)

		return data
	case CMD_MSG_U2GS_Ranking_Blacktemple:
		data, _ := READ_MSG_U2GS_Ranking_Blacktemple(bin)

		return data
	case CMD_MSG_GS2U_Ranking_Blacktemple_Result:
		data, _ := READ_MSG_GS2U_Ranking_Blacktemple_Result(bin)

		return data
	case CMD_MSG_GS2U_Activity_Arena_Ranking_info:
		data, _ := READ_MSG_GS2U_Activity_Arena_Ranking_info(bin)

		return data
	case CMD_MSG_U2GS_Activity_Arena_Ranking_Award:
		data, _ := READ_MSG_U2GS_Activity_Arena_Ranking_Award(bin)

		return data
	case CMD_MSG_GS2U_Activity_Arena_Ranking_Award_Result:
		data, _ := READ_MSG_GS2U_Activity_Arena_Ranking_Award_Result(bin)

		return data
	case CMD_MSG_U2GS_Activity_Arena_Ranking:
		data, _ := READ_MSG_U2GS_Activity_Arena_Ranking(bin)

		return data
	case CMD_MSG_GS2U_Activity_Arena_Ranking_Result:
		data, _ := READ_MSG_GS2U_Activity_Arena_Ranking_Result(bin)

		return data
	case CMD_MSG_U2GS_Player_Wrestling_Info:
		data, _ := READ_MSG_U2GS_Player_Wrestling_Info(bin)

		return data
	case CMD_MSG_GS2U_Wrestling_TargetPlayer:
		data, _ := READ_MSG_GS2U_Wrestling_TargetPlayer(bin)

		return data
	case CMD_MSG_GS2U_Player_Wrestling_Info_Result:
		data, _ := READ_MSG_GS2U_Player_Wrestling_Info_Result(bin)

		return data
	case CMD_MSG_U2GS_Wrestling_Reset:
		data, _ := READ_MSG_U2GS_Wrestling_Reset(bin)

		return data
	case CMD_MSG_GS2U_Wrestling_Reset_Result:
		data, _ := READ_MSG_GS2U_Wrestling_Reset_Result(bin)

		return data
	case CMD_MSG_U2GS_Wrestling_Dare:
		data, _ := READ_MSG_U2GS_Wrestling_Dare(bin)

		return data
	case CMD_MSG_GS2U_Wrestling_Dare_Result:
		data, _ := READ_MSG_GS2U_Wrestling_Dare_Result(bin)

		return data
	case CMD_MSG_U2GS_Wrestling_Dare_Reportserver:
		data, _ := READ_MSG_U2GS_Wrestling_Dare_Reportserver(bin)

		return data
	case CMD_MSG_GS2U_Wrestling_Dare_Reportserver_Result:
		data, _ := READ_MSG_GS2U_Wrestling_Dare_Reportserver_Result(bin)

		return data
	case CMD_MSG_U2GS_Get_WrestlingAward:
		data, _ := READ_MSG_U2GS_Get_WrestlingAward(bin)

		return data
	case CMD_MSG_GS2U_Get_WrestlingAward_Result:
		data, _ := READ_MSG_GS2U_Get_WrestlingAward_Result(bin)

		return data
	case CMD_MSG_U2GS_FortressInfo:
		data, _ := READ_MSG_U2GS_FortressInfo(bin)

		return data
	case CMD_MSG_FortressPlayerInfo:
		data, _ := READ_MSG_FortressPlayerInfo(bin)

		return data
	case CMD_MSG_FortressMsg:
		data, _ := READ_MSG_FortressMsg(bin)

		return data
	case CMD_MSG_GS2U_FortressInfo_Result:
		data, _ := READ_MSG_GS2U_FortressInfo_Result(bin)

		return data
	case CMD_MSG_U2GS_Fortress_Background:
		data, _ := READ_MSG_U2GS_Fortress_Background(bin)

		return data
	case CMD_MSG_GS2U_Fortress_Background_Result:
		data, _ := READ_MSG_GS2U_Fortress_Background_Result(bin)

		return data
	case CMD_MSG_U2GS_Fortress_SetMsg:
		data, _ := READ_MSG_U2GS_Fortress_SetMsg(bin)

		return data
	case CMD_MSG_GS2U_Fortress_PopupMsg_Result:
		data, _ := READ_MSG_GS2U_Fortress_PopupMsg_Result(bin)

		return data
	case CMD_MSG_U2GS_Fortress_Get_Box:
		data, _ := READ_MSG_U2GS_Fortress_Get_Box(bin)

		return data
	case CMD_MSG_GS2U_Fortress_Get_Box_Result:
		data, _ := READ_MSG_GS2U_Fortress_Get_Box_Result(bin)

		return data
	case CMD_MSG_U2GS_Fortress_Into_Player:
		data, _ := READ_MSG_U2GS_Fortress_Into_Player(bin)

		return data
	case CMD_MSG_GS2U_Fortress_Into_Player_Result:
		data, _ := READ_MSG_GS2U_Fortress_Into_Player_Result(bin)

		return data
	case CMD_MSG_U2GS_Fortress_GetPlayerHero_Exinfo:
		data, _ := READ_MSG_U2GS_Fortress_GetPlayerHero_Exinfo(bin)

		return data
	case CMD_MSG_GS2U_Fortress_GetPlayerHero_Exinfo_Result:
		data, _ := READ_MSG_GS2U_Fortress_GetPlayerHero_Exinfo_Result(bin)

		return data
	case CMD_MSG_U2GS_Fortress_GoodAndMsg:
		data, _ := READ_MSG_U2GS_Fortress_GoodAndMsg(bin)

		return data
	case CMD_MSG_GS2U_Fortress_GoodAndMsg_Result:
		data, _ := READ_MSG_GS2U_Fortress_GoodAndMsg_Result(bin)

		return data
	case CMD_MSG_U2GS_Fortress_GivePhysical:
		data, _ := READ_MSG_U2GS_Fortress_GivePhysical(bin)

		return data
	case CMD_MSG_GS2U_Fortress_GivePhysical_Result:
		data, _ := READ_MSG_GS2U_Fortress_GivePhysical_Result(bin)

		return data
	case CMD_MSG_U2GS_Fortress_Exchange:
		data, _ := READ_MSG_U2GS_Fortress_Exchange(bin)

		return data
	case CMD_MSG_GS2U_Fortress_Exchange_Result:
		data, _ := READ_MSG_GS2U_Fortress_Exchange_Result(bin)

		return data
	case CMD_MSG_U2GS_Seek_TargetTeam:
		data, _ := READ_MSG_U2GS_Seek_TargetTeam(bin)

		return data
	case CMD_MSG_GS2U_Seek_TargetTeam_Result:
		data, _ := READ_MSG_GS2U_Seek_TargetTeam_Result(bin)

		return data
	case CMD_MSG_SignAwardInfo:
		data, _ := READ_MSG_SignAwardInfo(bin)

		return data
	case CMD_MSG_SignNumAwardInfo:
		data, _ := READ_MSG_SignNumAwardInfo(bin)

		return data
	case CMD_MSG_U2GS_Get_Player_Sign_Info:
		data, _ := READ_MSG_U2GS_Get_Player_Sign_Info(bin)

		return data
	case CMD_MSG_GS2U_Get_Player_Sign_Info_Ret:
		data, _ := READ_MSG_GS2U_Get_Player_Sign_Info_Ret(bin)

		return data
	case CMD_MSG_U2GS_Get_Sign_Award:
		data, _ := READ_MSG_U2GS_Get_Sign_Award(bin)

		return data
	case CMD_MSG_GS2U_Get_Sign_Award_Result:
		data, _ := READ_MSG_GS2U_Get_Sign_Award_Result(bin)

		return data
	case CMD_MSG_U2GS_Get_Total_SignAward:
		data, _ := READ_MSG_U2GS_Get_Total_SignAward(bin)

		return data
	case CMD_MSG_GS2U_Get_Total_SignAward_Result:
		data, _ := READ_MSG_GS2U_Get_Total_SignAward_Result(bin)

		return data
	case CMD_MSG_LegendHot:
		data, _ := READ_MSG_LegendHot(bin)

		return data
	case CMD_MSG_U2GS_Get_Drunkery_Info:
		data, _ := READ_MSG_U2GS_Get_Drunkery_Info(bin)

		return data
	case CMD_MSG_GS2U_Get_Drunkery_Info_Ret:
		data, _ := READ_MSG_GS2U_Get_Drunkery_Info_Ret(bin)

		return data
	case CMD_MSG_U2GS_Drunkery_Lottery:
		data, _ := READ_MSG_U2GS_Drunkery_Lottery(bin)

		return data
	case CMD_MSG_LotteryShowInfo:
		data, _ := READ_MSG_LotteryShowInfo(bin)

		return data
	case CMD_MSG_GS2U_Drunkery_Lottery_Result:
		data, _ := READ_MSG_GS2U_Drunkery_Lottery_Result(bin)

		return data
	case CMD_MSG_U2GS_Select_Drunkery_Hero:
		data, _ := READ_MSG_U2GS_Select_Drunkery_Hero(bin)

		return data
	case CMD_MSG_GS2U_Select_Drunkery_Hero_Result:
		data, _ := READ_MSG_GS2U_Select_Drunkery_Hero_Result(bin)

		return data
	case CMD_MSG_U2GS_Get_Equip_Lottery_Info:
		data, _ := READ_MSG_U2GS_Get_Equip_Lottery_Info(bin)

		return data
	case CMD_MSG_GS2U_Get_Equip_Lottery_Info_Ret:
		data, _ := READ_MSG_GS2U_Get_Equip_Lottery_Info_Ret(bin)

		return data
	case CMD_MSG_U2GS_Equip_Lottery:
		data, _ := READ_MSG_U2GS_Equip_Lottery(bin)

		return data
	case CMD_MSG_GS2U_Equip_Lottery_Result:
		data, _ := READ_MSG_GS2U_Equip_Lottery_Result(bin)

		return data
	case CMD_MSG_GS2U_AcceptedQuest:
		data, _ := READ_MSG_GS2U_AcceptedQuest(bin)

		return data
	case CMD_MSG_GS2U_QuestList:
		data, _ := READ_MSG_GS2U_QuestList(bin)

		return data
	case CMD_MSG_U2GS_Quest_GetAward:
		data, _ := READ_MSG_U2GS_Quest_GetAward(bin)

		return data
	case CMD_MSG_GS2U_Quest_GetAward_Result:
		data, _ := READ_MSG_GS2U_Quest_GetAward_Result(bin)

		return data
	case CMD_MSG_GS2U_QuestDelete:
		data, _ := READ_MSG_GS2U_QuestDelete(bin)

		return data
	case CMD_MSG_ExploreQuest:
		data, _ := READ_MSG_ExploreQuest(bin)

		return data
	case CMD_MSG_GS2U_ExploreQuest_List:
		data, _ := READ_MSG_GS2U_ExploreQuest_List(bin)

		return data
	case CMD_MSG_ExploreQuest_Player:
		data, _ := READ_MSG_ExploreQuest_Player(bin)

		return data
	case CMD_MSG_GS2U_ExploreQuest_Player_List:
		data, _ := READ_MSG_GS2U_ExploreQuest_Player_List(bin)

		return data
	case CMD_MSG_U2GS_ExploreQuest_Accept:
		data, _ := READ_MSG_U2GS_ExploreQuest_Accept(bin)

		return data
	case CMD_MSG_GS2U_ExploreQuest_Accept_Result:
		data, _ := READ_MSG_GS2U_ExploreQuest_Accept_Result(bin)

		return data
	case CMD_MSG_U2GS_ExploreQuest_Submit:
		data, _ := READ_MSG_U2GS_ExploreQuest_Submit(bin)

		return data
	case CMD_MSG_GS2U_ExploreQuest_Submit_Result:
		data, _ := READ_MSG_GS2U_ExploreQuest_Submit_Result(bin)

		return data
	case CMD_MSG_U2GS_ActiveDegree_Get:
		data, _ := READ_MSG_U2GS_ActiveDegree_Get(bin)

		return data
	case CMD_MSG_GS2U_ActiveDegree_Get_Result:
		data, _ := READ_MSG_GS2U_ActiveDegree_Get_Result(bin)

		return data
	case CMD_MSG_Artifact_Node:
		data, _ := READ_MSG_Artifact_Node(bin)

		return data
	case CMD_MSG_Job_Artifact:
		data, _ := READ_MSG_Job_Artifact(bin)

		return data
	case CMD_MSG_GS2U_Artifact_List:
		data, _ := READ_MSG_GS2U_Artifact_List(bin)

		return data
	case CMD_MSG_U2GS_Artifact_UpLevel:
		data, _ := READ_MSG_U2GS_Artifact_UpLevel(bin)

		return data
	case CMD_MSG_GS2U_Artifact_UpLevel_Result:
		data, _ := READ_MSG_GS2U_Artifact_UpLevel_Result(bin)

		return data
	case CMD_MSG_U2GS_Artifact_Reset:
		data, _ := READ_MSG_U2GS_Artifact_Reset(bin)

		return data
	case CMD_MSG_GS2U_Artifact_Reset_Result:
		data, _ := READ_MSG_GS2U_Artifact_Reset_Result(bin)

		return data
	case CMD_MSG_GS2U_DailyTarget:
		data, _ := READ_MSG_GS2U_DailyTarget(bin)

		return data
	case CMD_MSG_GS2U_DailyTargetList:
		data, _ := READ_MSG_GS2U_DailyTargetList(bin)

		return data
	case CMD_MSG_GS2U_FunCount_Back:
		data, _ := READ_MSG_GS2U_FunCount_Back(bin)

		return data
	case CMD_MSG_GS2U_FunCount_Back_List:
		data, _ := READ_MSG_GS2U_FunCount_Back_List(bin)

		return data
	case CMD_MSG_FunCount_Back:
		data, _ := READ_MSG_FunCount_Back(bin)

		return data
	case CMD_MSG_U2GS_FunCount_Back_Get:
		data, _ := READ_MSG_U2GS_FunCount_Back_Get(bin)

		return data
	case CMD_MSG_GS2U_FunCount_Back_Get_Result:
		data, _ := READ_MSG_GS2U_FunCount_Back_Get_Result(bin)

		return data
	case CMD_MSG_WaresInfo:
		data, _ := READ_MSG_WaresInfo(bin)

		return data
	case CMD_MSG_U2GS_RequestWaresList:
		data, _ := READ_MSG_U2GS_RequestWaresList(bin)

		return data
	case CMD_MSG_GS2U_ResponseWaresList:
		data, _ := READ_MSG_GS2U_ResponseWaresList(bin)

		return data
	case CMD_MSG_U2GS_RequestBuyWare:
		data, _ := READ_MSG_U2GS_RequestBuyWare(bin)

		return data
	case CMD_MSG_GS2U_ResponseBuyWare:
		data, _ := READ_MSG_GS2U_ResponseBuyWare(bin)

		return data
	case CMD_MSG_GS2U_BuyWareSucess:
		data, _ := READ_MSG_GS2U_BuyWareSucess(bin)

		return data
	case CMD_MSG_GS2U_HasFirstRechargeAward:
		data, _ := READ_MSG_GS2U_HasFirstRechargeAward(bin)

		return data
	case CMD_MSG_U2GS_QueryFirstRechargeAward:
		data, _ := READ_MSG_U2GS_QueryFirstRechargeAward(bin)

		return data
	case CMD_MSG_GS2U_FirstRechargeAward:
		data, _ := READ_MSG_GS2U_FirstRechargeAward(bin)

		return data
	case CMD_MSG_U2GS_GetFirstRechargeAward:
		data, _ := READ_MSG_U2GS_GetFirstRechargeAward(bin)

		return data
	case CMD_MSG_GS2U_GetFirstRechargeAwardResult:
		data, _ := READ_MSG_GS2U_GetFirstRechargeAwardResult(bin)

		return data
	case CMD_MSG_U2GS_Get_Vip_Reward:
		data, _ := READ_MSG_U2GS_Get_Vip_Reward(bin)

		return data
	case CMD_MSG_GS2U_Get_Vip_Reward_Result:
		data, _ := READ_MSG_GS2U_Get_Vip_Reward_Result(bin)

		return data
	case CMD_MSG_U2GS_RequestMonthCardInfo:
		data, _ := READ_MSG_U2GS_RequestMonthCardInfo(bin)

		return data
	case CMD_MSG_GS2U_ResponseMonthCardInfo:
		data, _ := READ_MSG_GS2U_ResponseMonthCardInfo(bin)

		return data
	case CMD_MSG_U2GS_RequestGetMonthCardGold:
		data, _ := READ_MSG_U2GS_RequestGetMonthCardGold(bin)

		return data
	case CMD_MSG_GS2U_ResponseGetMonthCardGold:
		data, _ := READ_MSG_GS2U_ResponseGetMonthCardGold(bin)

		return data
	case CMD_MSG_PlayerGrowthInfo:
		data, _ := READ_MSG_PlayerGrowthInfo(bin)

		return data
	case CMD_MSG_U2GS_RequestGrowthFund:
		data, _ := READ_MSG_U2GS_RequestGrowthFund(bin)

		return data
	case CMD_MSG_GS2U_ResponseGrowthFund:
		data, _ := READ_MSG_GS2U_ResponseGrowthFund(bin)

		return data
	case CMD_MSG_U2GS_RequestGetGrowthFund:
		data, _ := READ_MSG_U2GS_RequestGetGrowthFund(bin)

		return data
	case CMD_MSG_GS2U_ResponseGetGrowthFund:
		data, _ := READ_MSG_GS2U_ResponseGetGrowthFund(bin)

		return data
	case CMD_MSG_U2GS_CheckOrder:
		data, _ := READ_MSG_U2GS_CheckOrder(bin)

		return data
	case CMD_MSG_TreasureLog:
		data, _ := READ_MSG_TreasureLog(bin)

		return data
	case CMD_MSG_GS2U_TreasureInfo:
		data, _ := READ_MSG_GS2U_TreasureInfo(bin)

		return data
	case CMD_MSG_U2GS_Get_Treasure_Info:
		data, _ := READ_MSG_U2GS_Get_Treasure_Info(bin)

		return data
	case CMD_MSG_GS2U_Get_Treasure_Info_Ret:
		data, _ := READ_MSG_GS2U_Get_Treasure_Info_Ret(bin)

		return data
	case CMD_MSG_U2GS_Do_Treasure:
		data, _ := READ_MSG_U2GS_Do_Treasure(bin)

		return data
	case CMD_MSG_GS2U_Do_Treasure_Result:
		data, _ := READ_MSG_GS2U_Do_Treasure_Result(bin)

		return data
	case CMD_MSG_U2GS_Clear_Treasure_Log:
		data, _ := READ_MSG_U2GS_Clear_Treasure_Log(bin)

		return data
	case CMD_MSG_GS2U_Clear_Treasure_Log_Result:
		data, _ := READ_MSG_GS2U_Clear_Treasure_Log_Result(bin)

		return data
	case CMD_MSG_GS2U_Battlefield_Info:
		data, _ := READ_MSG_GS2U_Battlefield_Info(bin)

		return data
	case CMD_MSG_U2GS_Battlefield_SignUp:
		data, _ := READ_MSG_U2GS_Battlefield_SignUp(bin)

		return data
	case CMD_MSG_GS2U_Battlefield_SignUp_Result:
		data, _ := READ_MSG_GS2U_Battlefield_SignUp_Result(bin)

		return data
	case CMD_MSG_U2GS_Battlefield_Into:
		data, _ := READ_MSG_U2GS_Battlefield_Into(bin)

		return data
	case CMD_MSG_GS2U_Battlefield_Into_Result:
		data, _ := READ_MSG_GS2U_Battlefield_Into_Result(bin)

		return data
	case CMD_MSG_U2GS_Battlefield_CloseScene:
		data, _ := READ_MSG_U2GS_Battlefield_CloseScene(bin)

		return data
	case CMD_MSG_GS2U_Battlefield_CloseScene:
		data, _ := READ_MSG_GS2U_Battlefield_CloseScene(bin)

		return data
	case CMD_MSG_U2GS_Battlefield_CloseDanMu:
		data, _ := READ_MSG_U2GS_Battlefield_CloseDanMu(bin)

		return data
	case CMD_MSG_GS2U_Battlefield_CloseDanMu:
		data, _ := READ_MSG_GS2U_Battlefield_CloseDanMu(bin)

		return data
	case CMD_MSG_GS2U_Battlefield_AllIntegral:
		data, _ := READ_MSG_GS2U_Battlefield_AllIntegral(bin)

		return data
	case CMD_MSG_U2GS_Battlefield_AotoSelectLine:
		data, _ := READ_MSG_U2GS_Battlefield_AotoSelectLine(bin)

		return data
	case CMD_MSG_GS2U_Battlefield_AotoSelectLine:
		data, _ := READ_MSG_GS2U_Battlefield_AotoSelectLine(bin)

		return data
	case CMD_MSG_U2GS_Battlefield_SceneModify:
		data, _ := READ_MSG_U2GS_Battlefield_SceneModify(bin)

		return data
	case CMD_MSG_U2GS_Battlefield_GetScene_Info:
		data, _ := READ_MSG_U2GS_Battlefield_GetScene_Info(bin)

		return data
	case CMD_MSG_Battlefield_GunPlayer:
		data, _ := READ_MSG_Battlefield_GunPlayer(bin)

		return data
	case CMD_MSG_GS2U_Battlefield_LinePlayer:
		data, _ := READ_MSG_GS2U_Battlefield_LinePlayer(bin)

		return data
	case CMD_MSG_GS2U_Battlefield_Head_HpV:
		data, _ := READ_MSG_GS2U_Battlefield_Head_HpV(bin)

		return data
	case CMD_MSG_GS2U_Battlefield_GetScene_Info:
		data, _ := READ_MSG_GS2U_Battlefield_GetScene_Info(bin)

		return data
	case CMD_MSG_GS2U_Battlefield_Modify_Status:
		data, _ := READ_MSG_GS2U_Battlefield_Modify_Status(bin)

		return data
	case CMD_MSG_GS2U_Battlefield_WarInfo:
		data, _ := READ_MSG_GS2U_Battlefield_WarInfo(bin)

		return data
	case CMD_MSG_GS2U_Battlefield_PassLine:
		data, _ := READ_MSG_GS2U_Battlefield_PassLine(bin)

		return data
	case CMD_MSG_GS2U_Battlefield_Modify_Rank:
		data, _ := READ_MSG_GS2U_Battlefield_Modify_Rank(bin)

		return data
	case CMD_MSG_GS2U_Battlefield_Modify_GunRank:
		data, _ := READ_MSG_GS2U_Battlefield_Modify_GunRank(bin)

		return data
	case CMD_MSG_GS2U_Battlefield_Msg:
		data, _ := READ_MSG_GS2U_Battlefield_Msg(bin)

		return data
	case CMD_MSG_U2GS_Battlefield_SelectLine:
		data, _ := READ_MSG_U2GS_Battlefield_SelectLine(bin)

		return data
	case CMD_MSG_GS2U_Battlefield_SelectLine_Result:
		data, _ := READ_MSG_GS2U_Battlefield_SelectLine_Result(bin)

		return data
	case CMD_MSG_U2GS_Battlefield_Rank:
		data, _ := READ_MSG_U2GS_Battlefield_Rank(bin)

		return data
	case CMD_MSG_Battlefield_RankPlayer:
		data, _ := READ_MSG_Battlefield_RankPlayer(bin)

		return data
	case CMD_MSG_GS2U_Battlefield_Rank_Result:
		data, _ := READ_MSG_GS2U_Battlefield_Rank_Result(bin)

		return data
	case CMD_MSG_GS2U_Battlefield_End:
		data, _ := READ_MSG_GS2U_Battlefield_End(bin)

		return data
	case CMD_MSG_GS2U_WorldBoss_Info:
		data, _ := READ_MSG_GS2U_WorldBoss_Info(bin)

		return data
	case CMD_MSG_GS2U_WorldBoss_Fight:
		data, _ := READ_MSG_GS2U_WorldBoss_Fight(bin)

		return data
	case CMD_MSG_GS2U_WorldBoss_Rank:
		data, _ := READ_MSG_GS2U_WorldBoss_Rank(bin)

		return data
	case CMD_MSG_GS2U_WorldBoss_Other_OpenBox:
		data, _ := READ_MSG_GS2U_WorldBoss_Other_OpenBox(bin)

		return data
	case CMD_MSG_U2GS_WorldBoss_GetScene:
		data, _ := READ_MSG_U2GS_WorldBoss_GetScene(bin)

		return data
	case CMD_MSG_WorldBoss_LastRank:
		data, _ := READ_MSG_WorldBoss_LastRank(bin)

		return data
	case CMD_MSG_GS2U_WorldBoss_GetScene_before:
		data, _ := READ_MSG_GS2U_WorldBoss_GetScene_before(bin)

		return data
	case CMD_MSG_GS2U_WorldBoss_GetScene_ing:
		data, _ := READ_MSG_GS2U_WorldBoss_GetScene_ing(bin)

		return data
	case CMD_MSG_GS2U_WorldBoss_GetScene_end:
		data, _ := READ_MSG_GS2U_WorldBoss_GetScene_end(bin)

		return data
	case CMD_MSG_U2GS_WorldBoss_CloseScene:
		data, _ := READ_MSG_U2GS_WorldBoss_CloseScene(bin)

		return data
	case CMD_MSG_U2GS_WorldBoss_Dare:
		data, _ := READ_MSG_U2GS_WorldBoss_Dare(bin)

		return data

	case CMD_MSG_GS2U_WorldBoss_Dare_Aoto:
		data, _ := READ_MSG_GS2U_WorldBoss_Dare_Aoto(bin)

		return data
	case CMD_MSG_U2GS_WorldBoss_AotoFight:
		data, _ := READ_MSG_U2GS_WorldBoss_AotoFight(bin)

		return data
	case CMD_MSG_GS2U_WorldBoss_AotoFight:
		data, _ := READ_MSG_GS2U_WorldBoss_AotoFight(bin)

		return data
	case CMD_MSG_U2GS_WorldBoss_relive:
		data, _ := READ_MSG_U2GS_WorldBoss_relive(bin)

		return data
	case CMD_MSG_GS2U_WorldBoss_relive:
		data, _ := READ_MSG_GS2U_WorldBoss_relive(bin)

		return data
	case CMD_MSG_U2GS_WorldBoss_openbox:
		data, _ := READ_MSG_U2GS_WorldBoss_openbox(bin)

		return data
	case CMD_MSG_GS2U_WorldBoss_openbox:
		data, _ := READ_MSG_GS2U_WorldBoss_openbox(bin)

		return data
	case CMD_MSG_Show_Item:
		data, _ := READ_MSG_Show_Item(bin)

		return data
	case CMD_MSG_U2GS_Player_Chat_Send:
		data, _ := READ_MSG_U2GS_Player_Chat_Send(bin)

		return data
	case CMD_MSG_GS2U_Player_Chat_Send_Result:
		data, _ := READ_MSG_GS2U_Player_Chat_Send_Result(bin)

		return data
	case CMD_MSG_GS2U_Player_Chat_Receive:
		data, _ := READ_MSG_GS2U_Player_Chat_Receive(bin)

		return data
	case CMD_MSG_U2GS_Player_Chat_Sound_Send:
		data, _ := READ_MSG_U2GS_Player_Chat_Sound_Send(bin)

		return data
	case CMD_MSG_GS2U_Player_Chat_Send_Sound_Result:
		data, _ := READ_MSG_GS2U_Player_Chat_Send_Sound_Result(bin)

		return data
	case CMD_MSG_GS2U_Player_Chat_Sound_Receive:
		data, _ := READ_MSG_GS2U_Player_Chat_Sound_Receive(bin)

		return data
	case CMD_MSG_U2GS_Get_Sound_Guid:
		data, _ := READ_MSG_U2GS_Get_Sound_Guid(bin)

		return data
	case CMD_MSG_GS2U_Get_Sound_Guid_Ret:
		data, _ := READ_MSG_GS2U_Get_Sound_Guid_Ret(bin)

		return data
	case CMD_MSG_U2GS_Chat_Item_Show_Info:
		data, _ := READ_MSG_U2GS_Chat_Item_Show_Info(bin)

		return data
	case CMD_MSG_GS2U_Chat_Item_Show_Info_Result:
		data, _ := READ_MSG_GS2U_Chat_Item_Show_Info_Result(bin)

		return data
	case CMD_MSG_GS2U_System_Msg:
		data, _ := READ_MSG_GS2U_System_Msg(bin)

		return data
	case CMD_MSG_OfflineChatInfo:
		data, _ := READ_MSG_OfflineChatInfo(bin)

		return data
	case CMD_MSG_GS2U_Offline_Chat_Info:
		data, _ := READ_MSG_GS2U_Offline_Chat_Info(bin)

		return data
	case CMD_MSG_Friend_Info:
		data, _ := READ_MSG_Friend_Info(bin)

		return data
	case CMD_MSG_GS2U_Player_Friend_Info:
		data, _ := READ_MSG_GS2U_Player_Friend_Info(bin)

		return data
	case CMD_MSG_GS2U_Player_Add_Friend_Info:
		data, _ := READ_MSG_GS2U_Player_Add_Friend_Info(bin)

		return data
	case CMD_MSG_GS2U_FriendRequestInfo:
		data, _ := READ_MSG_GS2U_FriendRequestInfo(bin)

		return data
	case CMD_MSG_U2GS_Add_Friend:
		data, _ := READ_MSG_U2GS_Add_Friend(bin)

		return data
	case CMD_MSG_U2GS_Add_Friend_By_Name:
		data, _ := READ_MSG_U2GS_Add_Friend_By_Name(bin)

		return data
	case CMD_MSG_GS2U_Add_Friend_Result:
		data, _ := READ_MSG_GS2U_Add_Friend_Result(bin)

		return data
	case CMD_MSG_U2GS_Reply_Add_Friend:
		data, _ := READ_MSG_U2GS_Reply_Add_Friend(bin)

		return data
	case CMD_MSG_GS2U_Reply_Add_Friend_Result:
		data, _ := READ_MSG_GS2U_Reply_Add_Friend_Result(bin)

		return data
	case CMD_MSG_GS2U_Tell_Source_Add_Result:
		data, _ := READ_MSG_GS2U_Tell_Source_Add_Result(bin)

		return data
	case CMD_MSG_U2GS_Del_Friend:
		data, _ := READ_MSG_U2GS_Del_Friend(bin)

		return data
	case CMD_MSG_GS2U_Del_Friend_Result:
		data, _ := READ_MSG_GS2U_Del_Friend_Result(bin)

		return data
	case CMD_MSG_GS2U_Tell_Target_Be_Delete:
		data, _ := READ_MSG_GS2U_Tell_Target_Be_Delete(bin)

		return data
	case CMD_MSG_GS2U_Del_Friend_Info:
		data, _ := READ_MSG_GS2U_Del_Friend_Info(bin)

		return data
	case CMD_MSG_U2GS_Recommend_Friend:
		data, _ := READ_MSG_U2GS_Recommend_Friend(bin)

		return data
	case CMD_MSG_GS2U_Recommend_Friend_Result:
		data, _ := READ_MSG_GS2U_Recommend_Friend_Result(bin)

		return data
	case CMD_MSG_U2GS_OneKey_AddFriend:
		data, _ := READ_MSG_U2GS_OneKey_AddFriend(bin)

		return data
	case CMD_MSG_GS2U_OneKey_AddFriend_Result:
		data, _ := READ_MSG_GS2U_OneKey_AddFriend_Result(bin)

		return data
	case CMD_MSG_U2GS_OneKey_Reply:
		data, _ := READ_MSG_U2GS_OneKey_Reply(bin)

		return data
	case CMD_MSG_GS2U_OneKey_Reply_Result:
		data, _ := READ_MSG_GS2U_OneKey_Reply_Result(bin)

		return data
	case CMD_MSG_RecvPhyStrengthInfo:
		data, _ := READ_MSG_RecvPhyStrengthInfo(bin)

		return data
	case CMD_MSG_GS2U_Player_Recv_PhyStrength_Info:
		data, _ := READ_MSG_GS2U_Player_Recv_PhyStrength_Info(bin)

		return data
	case CMD_MSG_GS2U_Player_Give_PhyStrength_Info:
		data, _ := READ_MSG_GS2U_Player_Give_PhyStrength_Info(bin)

		return data
	case CMD_MSG_U2GS_Give_Friend_PhyStrength:
		data, _ := READ_MSG_U2GS_Give_Friend_PhyStrength(bin)

		return data
	case CMD_MSG_GS2U_Give_Friend_PhyStrength_Result:
		data, _ := READ_MSG_GS2U_Give_Friend_PhyStrength_Result(bin)

		return data
	case CMD_MSG_U2GS_OneKey_Give_PhyStrength:
		data, _ := READ_MSG_U2GS_OneKey_Give_PhyStrength(bin)

		return data
	case CMD_MSG_GS2U_OneKey_Give_PhyStrength_Result:
		data, _ := READ_MSG_GS2U_OneKey_Give_PhyStrength_Result(bin)

		return data
	case CMD_MSG_U2GS_Get_Recv_PhyStrength:
		data, _ := READ_MSG_U2GS_Get_Recv_PhyStrength(bin)

		return data
	case CMD_MSG_GS2U_Get_Recv_PhyStrength_Result:
		data, _ := READ_MSG_GS2U_Get_Recv_PhyStrength_Result(bin)

		return data
	case CMD_MSG_U2GS_Hero_Support_LevelUp:
		data, _ := READ_MSG_U2GS_Hero_Support_LevelUp(bin)

		return data
	case CMD_MSG_GS2U_Hero_Support_LevelUp_Result:
		data, _ := READ_MSG_GS2U_Hero_Support_LevelUp_Result(bin)

		return data
	case CMD_MSG_Support_On:
		data, _ := READ_MSG_Support_On(bin)

		return data
	case CMD_MSG_U2GS_Hero_Support_Active_Location:
		data, _ := READ_MSG_U2GS_Hero_Support_Active_Location(bin)

		return data
	case CMD_MSG_GS2U_Hero_Support_Active_Location_Result:
		data, _ := READ_MSG_GS2U_Hero_Support_Active_Location_Result(bin)

		return data
	case CMD_MSG_U2GS_Hero_Support_NoActive_Location:
		data, _ := READ_MSG_U2GS_Hero_Support_NoActive_Location(bin)

		return data
	case CMD_MSG_GS2U_Hero_Support_NoActive_Location_Result:
		data, _ := READ_MSG_GS2U_Hero_Support_NoActive_Location_Result(bin)

		return data
	case CMD_MSG_MarketItem:
		data, _ := READ_MSG_MarketItem(bin)

		return data
	case CMD_MSG_MarketInfo:
		data, _ := READ_MSG_MarketInfo(bin)

		return data
	case CMD_MSG_U2GS_Get_Market_Sale_Info:
		data, _ := READ_MSG_U2GS_Get_Market_Sale_Info(bin)

		return data
	case CMD_MSG_MarketBuyInfo:
		data, _ := READ_MSG_MarketBuyInfo(bin)

		return data
	case CMD_MSG_MarketLimitSale:
		data, _ := READ_MSG_MarketLimitSale(bin)

		return data
	case CMD_MSG_GS2U_Get_Market_Sale_Info_Ret:
		data, _ := READ_MSG_GS2U_Get_Market_Sale_Info_Ret(bin)

		return data
	case CMD_MSG_U2GS_Market_Buy_Item:
		data, _ := READ_MSG_U2GS_Market_Buy_Item(bin)

		return data
	case CMD_MSG_GS2U_Market_Buy_Item_Result:
		data, _ := READ_MSG_GS2U_Market_Buy_Item_Result(bin)

		return data
	case CMD_MSG_GS2U_Update_Market_Sale_Info:
		data, _ := READ_MSG_GS2U_Update_Market_Sale_Info(bin)

		return data
	case CMD_MSG_U2GS_Set_NewHand_ID:
		data, _ := READ_MSG_U2GS_Set_NewHand_ID(bin)

		return data
	case CMD_MSG_GS2U_Set_NewHand_ID_Result:
		data, _ := READ_MSG_GS2U_Set_NewHand_ID_Result(bin)

		return data
	case CMD_MSG_GS2U_NewHand_Del_ID:
		data, _ := READ_MSG_GS2U_NewHand_Del_ID(bin)

		return data
	case CMD_MSG_GS2U_NewHand:
		data, _ := READ_MSG_GS2U_NewHand(bin)

		return data
	case CMD_MSG_GS2U_NewHand_List:
		data, _ := READ_MSG_GS2U_NewHand_List(bin)

		return data
	case CMD_MSG_Index_Award_Flag:
		data, _ := READ_MSG_Index_Award_Flag(bin)

		return data
	case CMD_MSG_U2GS_Get_HeroLotteryConsume_Info:
		data, _ := READ_MSG_U2GS_Get_HeroLotteryConsume_Info(bin)

		return data
	case CMD_MSG_GS2U_Get_HeroLotteryConsume_Info_Result:
		data, _ := READ_MSG_GS2U_Get_HeroLotteryConsume_Info_Result(bin)

		return data
	case CMD_MSG_U2GS_Get_HeroLotteryConsume_Award:
		data, _ := READ_MSG_U2GS_Get_HeroLotteryConsume_Award(bin)

		return data
	case CMD_MSG_GS2U_Get_HeroLotteryConsume_Award_Result:
		data, _ := READ_MSG_GS2U_Get_HeroLotteryConsume_Award_Result(bin)

		return data
	case CMD_MSG_U2GS_Get_GemLotteryConsume_Info:
		data, _ := READ_MSG_U2GS_Get_GemLotteryConsume_Info(bin)

		return data
	case CMD_MSG_GS2U_Get_GemLotteryConsume_Info_Result:
		data, _ := READ_MSG_GS2U_Get_GemLotteryConsume_Info_Result(bin)

		return data
	case CMD_MSG_U2GS_Get_GemLotteryConsume_Award:
		data, _ := READ_MSG_U2GS_Get_GemLotteryConsume_Award(bin)

		return data
	case CMD_MSG_GS2U_Get_GemLotteryConsume_Award_Result:
		data, _ := READ_MSG_GS2U_Get_GemLotteryConsume_Award_Result(bin)

		return data
	case CMD_MSG_GS2U_LoginActivity_State:
		data, _ := READ_MSG_GS2U_LoginActivity_State(bin)

		return data
	case CMD_MSG_U2GS_Get_LoginActivity_Info:
		data, _ := READ_MSG_U2GS_Get_LoginActivity_Info(bin)

		return data
	case CMD_MSG_GS2U_Get_LoginActivity_Info_Result:
		data, _ := READ_MSG_GS2U_Get_LoginActivity_Info_Result(bin)

		return data
	case CMD_MSG_U2GS_Get_LoginActivity_Award:
		data, _ := READ_MSG_U2GS_Get_LoginActivity_Award(bin)

		return data
	case CMD_MSG_GS2U_Get_LoginActivity_Award_Result:
		data, _ := READ_MSG_GS2U_Get_LoginActivity_Award_Result(bin)

		return data
	case CMD_MSG_GS2U_Open_Fight_Act_Status:
		data, _ := READ_MSG_GS2U_Open_Fight_Act_Status(bin)

		return data
	case CMD_MSG_U2GS_Get_Open_Fight_Activity_Info:
		data, _ := READ_MSG_U2GS_Get_Open_Fight_Activity_Info(bin)

		return data
	case CMD_MSG_Open_Fight_Rank_Info:
		data, _ := READ_MSG_Open_Fight_Rank_Info(bin)

		return data
	case CMD_MSG_GS2U_Get_Open_Fight_Activity_Info_Result:
		data, _ := READ_MSG_GS2U_Get_Open_Fight_Activity_Info_Result(bin)

		return data
	case CMD_MSG_U2GS_Get_Open_Fight_Award:
		data, _ := READ_MSG_U2GS_Get_Open_Fight_Award(bin)

		return data
	case CMD_MSG_GS2U_Get_Open_Fight_Award_Result:
		data, _ := READ_MSG_GS2U_Get_Open_Fight_Award_Result(bin)

		return data
	case CMD_MSG_WelfareAward_Info:
		data, _ := READ_MSG_WelfareAward_Info(bin)

		return data
	case CMD_MSG_WelfareActivity_Info:
		data, _ := READ_MSG_WelfareActivity_Info(bin)

		return data
	case CMD_MSG_Welfare_State:
		data, _ := READ_MSG_Welfare_State(bin)

		return data
	case CMD_MSG_U2GS_Get_Activity_Welfare_Info:
		data, _ := READ_MSG_U2GS_Get_Activity_Welfare_Info(bin)

		return data
	case CMD_MSG_GS2U_Get_Activity_Welfare_Info_Result:
		data, _ := READ_MSG_GS2U_Get_Activity_Welfare_Info_Result(bin)

		return data
	case CMD_MSG_GS2U_Welfare_Activity_State:
		data, _ := READ_MSG_GS2U_Welfare_Activity_State(bin)

		return data
	case CMD_MSG_U2GS_Get_Welfare_Activity_Award:
		data, _ := READ_MSG_U2GS_Get_Welfare_Activity_Award(bin)

		return data
	case CMD_MSG_GS2U_Get_Welfare_Activity_Award_Result:
		data, _ := READ_MSG_GS2U_Get_Welfare_Activity_Award_Result(bin)

		return data
	case CMD_MSG_SecondActivityInfo:
		data, _ := READ_MSG_SecondActivityInfo(bin)

		return data
	case CMD_MSG_U2GS_Get_SecondActivity_Info:
		data, _ := READ_MSG_U2GS_Get_SecondActivity_Info(bin)

		return data
	case CMD_MSG_GS2U_Get_SecondActivity_Info_Result:
		data, _ := READ_MSG_GS2U_Get_SecondActivity_Info_Result(bin)

		return data
	case CMD_MSG_U2GS_Get_SecondActivity_Award:
		data, _ := READ_MSG_U2GS_Get_SecondActivity_Award(bin)

		return data
	case CMD_MSG_GS2U_Get_SecondActivity_Award_Result:
		data, _ := READ_MSG_GS2U_Get_SecondActivity_Award_Result(bin)

		return data
	case CMD_MSG_LimitTime_Task_Flag:
		data, _ := READ_MSG_LimitTime_Task_Flag(bin)

		return data
	case CMD_MSG_GS2U_LimitTime_Task_Flag:
		data, _ := READ_MSG_GS2U_LimitTime_Task_Flag(bin)

		return data
	case CMD_MSG_CollectItemPlayerInfo:
		data, _ := READ_MSG_CollectItemPlayerInfo(bin)

		return data
	case CMD_MSG_GS2U_Send_CollectItemPlayerInfo:
		data, _ := READ_MSG_GS2U_Send_CollectItemPlayerInfo(bin)

		return data
	case CMD_MSG_U2GS_Get_CollectItem_Activity:
		data, _ := READ_MSG_U2GS_Get_CollectItem_Activity(bin)

		return data
	case CMD_MSG_GS2U_Get_CollectItem_Activity_Result:
		data, _ := READ_MSG_GS2U_Get_CollectItem_Activity_Result(bin)

		return data
	case CMD_MSG_U2GS_Get_CollectItem_Award:
		data, _ := READ_MSG_U2GS_Get_CollectItem_Award(bin)

		return data
	case CMD_MSG_GS2U_Get_CollectItem_Award_Result:
		data, _ := READ_MSG_GS2U_Get_CollectItem_Award_Result(bin)

		return data
	case CMD_MSG_ChangeItemActivity:
		data, _ := READ_MSG_ChangeItemActivity(bin)

		return data
	case CMD_MSG_ChangeItemPlayerInfo:
		data, _ := READ_MSG_ChangeItemPlayerInfo(bin)

		return data
	case CMD_MSG_GS2U_Send_ChangeItem_PlayerInfo:
		data, _ := READ_MSG_GS2U_Send_ChangeItem_PlayerInfo(bin)

		return data
	case CMD_MSG_U2GS_Get_ChangeItem_Activity:
		data, _ := READ_MSG_U2GS_Get_ChangeItem_Activity(bin)

		return data
	case CMD_MSG_GS2U_Get_ChangeItem_Activity_Result:
		data, _ := READ_MSG_GS2U_Get_ChangeItem_Activity_Result(bin)

		return data
	case CMD_MSG_U2GS_ChangeItem:
		data, _ := READ_MSG_U2GS_ChangeItem(bin)

		return data
	case CMD_MSG_GS2U_ChangeItem_Result:
		data, _ := READ_MSG_GS2U_ChangeItem_Result(bin)

		return data
	case CMD_MSG_Carnival_Activity_Flag:
		data, _ := READ_MSG_Carnival_Activity_Flag(bin)

		return data
	case CMD_MSG_GS2U_Carnival_Award_Info:
		data, _ := READ_MSG_GS2U_Carnival_Award_Info(bin)

		return data
	case CMD_MSG_U2GS_PlayerClick:
		data, _ := READ_MSG_U2GS_PlayerClick(bin)

		return data
	case CMD_MSG_U2GS_Get_Active_Code_Gift:
		data, _ := READ_MSG_U2GS_Get_Active_Code_Gift(bin)

		return data
	case CMD_MSG_GS2U_Get_Active_Code_Gift_Result:
		data, _ := READ_MSG_GS2U_Get_Active_Code_Gift_Result(bin)

		return data
	case CMD_MSG_GS2U_FullClient_Award_Flag:
		data, _ := READ_MSG_GS2U_FullClient_Award_Flag(bin)

		return data
	case CMD_MSG_U2GS_Get_FullClient_Award:
		data, _ := READ_MSG_U2GS_Get_FullClient_Award(bin)

		return data
	case CMD_MSG_GS2U_Get_FullClient_Award_Result:
		data, _ := READ_MSG_GS2U_Get_FullClient_Award_Result(bin)

		return data
	case CMD_MSG_U2GS_Under_Frame_Package:
		data, _ := READ_MSG_U2GS_Under_Frame_Package(bin)

		return data
	case CMD_MSG_GS2U_Under_Frame_Package_Result:
		data, _ := READ_MSG_GS2U_Under_Frame_Package_Result(bin)

		return data
	case CMD_MSG_GS2U_Question_State:
		data, _ := READ_MSG_GS2U_Question_State(bin)

		return data
	case CMD_MSG_QuestionInfo:
		data, _ := READ_MSG_QuestionInfo(bin)

		return data
	case CMD_MSG_U2GS_Get_Qustion_Info:
		data, _ := READ_MSG_U2GS_Get_Qustion_Info(bin)

		return data
	case CMD_MSG_GS2U_Get_Question_Info_Result:
		data, _ := READ_MSG_GS2U_Get_Question_Info_Result(bin)

		return data
	case CMD_MSG_U2GS_Answer_Question:
		data, _ := READ_MSG_U2GS_Answer_Question(bin)

		return data
	case CMD_MSG_GS2U_Answer_Question_Result:
		data, _ := READ_MSG_GS2U_Answer_Question_Result(bin)

		return data
	case CMD_MSG_Quest30_State:
		data, _ := READ_MSG_Quest30_State(bin)

		return data
	case CMD_MSG_GS2U_Quest30_Player_Info:
		data, _ := READ_MSG_GS2U_Quest30_Player_Info(bin)

		return data
	case CMD_MSG_U2GS_Get_Quest30_Award:
		data, _ := READ_MSG_U2GS_Get_Quest30_Award(bin)

		return data
	case CMD_MSG_GS2U_Get_Quest30_Award_Result:
		data, _ := READ_MSG_GS2U_Get_Quest30_Award_Result(bin)

		return data
	case CMD_MSG_Opentarget_State:
		data, _ := READ_MSG_Opentarget_State(bin)

		return data
	case CMD_MSG_GS2U_opentarget_Player_Info:
		data, _ := READ_MSG_GS2U_opentarget_Player_Info(bin)

		return data
	case CMD_MSG_U2GS_Get_opentarget_Award:
		data, _ := READ_MSG_U2GS_Get_opentarget_Award(bin)

		return data
	case CMD_MSG_GS2U_Get_opentarget_Award_Result:
		data, _ := READ_MSG_GS2U_Get_opentarget_Award_Result(bin)

		return data
	case CMD_MSG_U2GS_CS_Arena_Info:
		data, _ := READ_MSG_U2GS_CS_Arena_Info(bin)

		return data
	case CMD_MSG_CS_Arena_Player:
		data, _ := READ_MSG_CS_Arena_Player(bin)

		return data
	case CMD_MSG_GS2U_CS_Arena_Info:
		data, _ := READ_MSG_GS2U_CS_Arena_Info(bin)

		return data
	case CMD_MSG_U2GS_CS_Arena_Refresh:
		data, _ := READ_MSG_U2GS_CS_Arena_Refresh(bin)

		return data
	case CMD_MSG_GS2U_CS_Arena_Refresh:
		data, _ := READ_MSG_GS2U_CS_Arena_Refresh(bin)

		return data
	case CMD_MSG_U2GS_CS_Arena_OpenBox:
		data, _ := READ_MSG_U2GS_CS_Arena_OpenBox(bin)

		return data
	case CMD_MSG_GS2U_CS_Arena_OpenBox:
		data, _ := READ_MSG_GS2U_CS_Arena_OpenBox(bin)

		return data
	case CMD_MSG_U2GS_CS_Arena_BuyDareCount:
		data, _ := READ_MSG_U2GS_CS_Arena_BuyDareCount(bin)

		return data
	case CMD_MSG_GS2U_CS_Arena_BuyDareCount:
		data, _ := READ_MSG_GS2U_CS_Arena_BuyDareCount(bin)

		return data
	case CMD_MSG_U2GS_CS_Arena_War:
		data, _ := READ_MSG_U2GS_CS_Arena_War(bin)

		return data
	case CMD_MSG_GS2U_CS_Arena_War:
		data, _ := READ_MSG_GS2U_CS_Arena_War(bin)

		return data
	case CMD_MSG_GS2U_CS_Arena_War_FightResult:
		data, _ := READ_MSG_GS2U_CS_Arena_War_FightResult(bin)

		return data
	case CMD_MSG_U2GS_CS_Arena_Rank:
		data, _ := READ_MSG_U2GS_CS_Arena_Rank(bin)

		return data
	case CMD_MSG_GS2U_CS_Arena_Rank:
		data, _ := READ_MSG_GS2U_CS_Arena_Rank(bin)

		return data
	case CMD_MSG_TerritoryPositionInfo:
		data, _ := READ_MSG_TerritoryPositionInfo(bin)

		return data
	case CMD_MSG_U2GS_RequestTerritoryInfo:
		data, _ := READ_MSG_U2GS_RequestTerritoryInfo(bin)

		return data
	case CMD_MSG_GS2U_ResponseTerritoryInfo:
		data, _ := READ_MSG_GS2U_ResponseTerritoryInfo(bin)

		return data
	case CMD_MSG_U2GS_RequestOpenTerritoryArea:
		data, _ := READ_MSG_U2GS_RequestOpenTerritoryArea(bin)

		return data
	case CMD_MSG_GS2U_ReponseOpenTerritoryAreaResult:
		data, _ := READ_MSG_GS2U_ReponseOpenTerritoryAreaResult(bin)

		return data
	case CMD_MSG_U2GS_RefreshPositionPlayer:
		data, _ := READ_MSG_U2GS_RefreshPositionPlayer(bin)

		return data
	case CMD_MSG_GS2U_RefreshPositionPlayerResult:
		data, _ := READ_MSG_GS2U_RefreshPositionPlayerResult(bin)

		return data
	case CMD_MSG_GS2U_TerritoryPositionChange:
		data, _ := READ_MSG_GS2U_TerritoryPositionChange(bin)

		return data
	case CMD_MSG_U2GS_TerritoryPositionFight:
		data, _ := READ_MSG_U2GS_TerritoryPositionFight(bin)

		return data
	case CMD_MSG_GS2U_TerritoryPositionFightInfo:
		data, _ := READ_MSG_GS2U_TerritoryPositionFightInfo(bin)

		return data
	case CMD_MSG_U2GS_TerritoryPositionFightInfoValidate:
		data, _ := READ_MSG_U2GS_TerritoryPositionFightInfoValidate(bin)

		return data
	case CMD_MSG_GS2U_TerritoryPositionFightResult:
		data, _ := READ_MSG_GS2U_TerritoryPositionFightResult(bin)

		return data
	case CMD_MSG_ArenaHeroHurt:
		data, _ := READ_MSG_ArenaHeroHurt(bin)

		return data
	case CMD_MSG_GS2U_TerritoryPositionFightPlayerResult:
		data, _ := READ_MSG_GS2U_TerritoryPositionFightPlayerResult(bin)

		return data
	case CMD_MSG_TerritoryHistory:
		data, _ := READ_MSG_TerritoryHistory(bin)

		return data
	case CMD_MSG_U2GS_RequestTerritoryHistory:
		data, _ := READ_MSG_U2GS_RequestTerritoryHistory(bin)

		return data
	case CMD_MSG_GS2U_ResponseTerritoryHistory:
		data, _ := READ_MSG_GS2U_ResponseTerritoryHistory(bin)

		return data
	case CMD_MSG_U2GS_RequestOpenTerritoryBox:
		data, _ := READ_MSG_U2GS_RequestOpenTerritoryBox(bin)

		return data
	case CMD_MSG_GS2U_ResponseOpenTerritoryBox:
		data, _ := READ_MSG_GS2U_ResponseOpenTerritoryBox(bin)

		return data
	case CMD_MSG_U2GS_QuickClearTerritoryPosition:
		data, _ := READ_MSG_U2GS_QuickClearTerritoryPosition(bin)

		return data
	case CMD_MSG_GS2U_QuickClearTerritoryPositionResult:
		data, _ := READ_MSG_GS2U_QuickClearTerritoryPositionResult(bin)

		return data
	case CMD_MSG_GS2U_IOS_FeatureLimit:
		data, _ := READ_MSG_GS2U_IOS_FeatureLimit(bin)

		return data
	case CMD_MSG_BlackShopInfo:
		data, _ := READ_MSG_BlackShopInfo(bin)

		return data
	case CMD_MSG_U2GS_Get_BlackShop_BuyInfo:
		data, _ := READ_MSG_U2GS_Get_BlackShop_BuyInfo(bin)

		return data
	case CMD_MSG_GS2U_Get_BlackShop_BuyInfo_Result:
		data, _ := READ_MSG_GS2U_Get_BlackShop_BuyInfo_Result(bin)

		return data
	case CMD_MSG_U2GS_Buy_BlackShop_Item:
		data, _ := READ_MSG_U2GS_Buy_BlackShop_Item(bin)

		return data
	case CMD_MSG_GS2U_Buy_BlackShop_Item_Result:
		data, _ := READ_MSG_GS2U_Buy_BlackShop_Item_Result(bin)

		return data
	case CMD_MSG_U2GS_Get_Consume_Rank:
		data, _ := READ_MSG_U2GS_Get_Consume_Rank(bin)

		return data
	case CMD_MSG_GS2U_Comsume_Rank_List:
		data, _ := READ_MSG_GS2U_Comsume_Rank_List(bin)

		return data
	case CMD_MSG_U2GS_Get_MonthCard_ExtraAward:
		data, _ := READ_MSG_U2GS_Get_MonthCard_ExtraAward(bin)

		return data
	case CMD_MSG_GS2U_Get_MonthCard_ExtraAward_Result:
		data, _ := READ_MSG_GS2U_Get_MonthCard_ExtraAward_Result(bin)

		return data
	case CMD_MSG_ShowLimitLotteryInfo:
		data, _ := READ_MSG_ShowLimitLotteryInfo(bin)

		return data
	case CMD_MSG_LimitLotteryScoreAwardInfo:
		data, _ := READ_MSG_LimitLotteryScoreAwardInfo(bin)

		return data
	case CMD_MSG_LimitLotteryActivityInfo:
		data, _ := READ_MSG_LimitLotteryActivityInfo(bin)

		return data
	case CMD_MSG_GS2U_LimitLottery_PlayerInfo:
		data, _ := READ_MSG_GS2U_LimitLottery_PlayerInfo(bin)

		return data
	case CMD_MSG_U2GS_Get_LimitLotteryActivity_Info:
		data, _ := READ_MSG_U2GS_Get_LimitLotteryActivity_Info(bin)

		return data
	case CMD_MSG_GS2U_Get_LimitLottery_Info_Result:
		data, _ := READ_MSG_GS2U_Get_LimitLottery_Info_Result(bin)

		return data
	case CMD_MSG_U2GS_LimitLottery_Operate:
		data, _ := READ_MSG_U2GS_LimitLottery_Operate(bin)

		return data
	case CMD_MSG_GS2U_LimitLottery_Operate_Result:
		data, _ := READ_MSG_GS2U_LimitLottery_Operate_Result(bin)

		return data
	case CMD_MSG_OverlordPlayerInfo:
		data, _ := READ_MSG_OverlordPlayerInfo(bin)

		return data
	case CMD_MSG_OverlordHistoryInfo:
		data, _ := READ_MSG_OverlordHistoryInfo(bin)

		return data
	case CMD_MSG_OverlordQualifyingHistoryInfo:
		data, _ := READ_MSG_OverlordQualifyingHistoryInfo(bin)

		return data
	case CMD_MSG_U2GS_RequestOverlordHistory:
		data, _ := READ_MSG_U2GS_RequestOverlordHistory(bin)

		return data
	case CMD_MSG_GS2U_ResponseOverlordHistoryList:
		data, _ := READ_MSG_GS2U_ResponseOverlordHistoryList(bin)

		return data
	case CMD_MSG_U2GS_RequestOverlordQualifyingHistory:
		data, _ := READ_MSG_U2GS_RequestOverlordQualifyingHistory(bin)

		return data
	case CMD_MSG_GS2U_ResponseOverlordQualifyingHistoryList:
		data, _ := READ_MSG_GS2U_ResponseOverlordQualifyingHistoryList(bin)

		return data
	case CMD_MSG_U2GS_RequestOverlordFightInfo:
		data, _ := READ_MSG_U2GS_RequestOverlordFightInfo(bin)

		return data
	case CMD_MSG_GS2U_ResponseOverlordFightInfo:
		data, _ := READ_MSG_GS2U_ResponseOverlordFightInfo(bin)

		return data
	case CMD_MSG_U2GS_RequestOverlordSignUp:
		data, _ := READ_MSG_U2GS_RequestOverlordSignUp(bin)

		return data
	case CMD_MSG_GS2U_ResponseOverlordSignUpResult:
		data, _ := READ_MSG_GS2U_ResponseOverlordSignUpResult(bin)

		return data
	case CMD_MSG_U2GS_RequestOverlordInfo:
		data, _ := READ_MSG_U2GS_RequestOverlordInfo(bin)

		return data
	case CMD_MSG_GS2U_OverlordBeforeInfo:
		data, _ := READ_MSG_GS2U_OverlordBeforeInfo(bin)

		return data
	case CMD_MSG_GS2U_OverlordSelectiveMatchInfo:
		data, _ := READ_MSG_GS2U_OverlordSelectiveMatchInfo(bin)

		return data
	case CMD_MSG_GS2U_OverlordQualifyingBeforeInfo:
		data, _ := READ_MSG_GS2U_OverlordQualifyingBeforeInfo(bin)

		return data
	case CMD_MSG_U2GS_RequestOverlordQualifyingInfo:
		data, _ := READ_MSG_U2GS_RequestOverlordQualifyingInfo(bin)

		return data
	case CMD_MSG_OverlordQualifyingMatching:
		data, _ := READ_MSG_OverlordQualifyingMatching(bin)

		return data
	case CMD_MSG_GS2U_OverlordQualifyingInfo:
		data, _ := READ_MSG_GS2U_OverlordQualifyingInfo(bin)

		return data
	case CMD_MSG_GS2U_OverlordCompletedInfo:
		data, _ := READ_MSG_GS2U_OverlordCompletedInfo(bin)

		return data
	case CMD_MSG_U2GS_OverlordSupportPlayer:
		data, _ := READ_MSG_U2GS_OverlordSupportPlayer(bin)

		return data
	case CMD_MSG_GS2U_OverlordSupportPlayerResult:
		data, _ := READ_MSG_GS2U_OverlordSupportPlayerResult(bin)

		return data
	case CMD_MSG_U2GS_RequestOverlordPlayerInfo:
		data, _ := READ_MSG_U2GS_RequestOverlordPlayerInfo(bin)

		return data
	case CMD_MSG_GS2U_ResponseOverlordPlayerInfo:
		data, _ := READ_MSG_GS2U_ResponseOverlordPlayerInfo(bin)

		return data
	case CMD_MSG_U2GS_DiggCrossOverlord:
		data, _ := READ_MSG_U2GS_DiggCrossOverlord(bin)

		return data
	case CMD_MSG_GS2U_DiggCrossOverlordResult:
		data, _ := READ_MSG_GS2U_DiggCrossOverlordResult(bin)

		return data
	case CMD_MSG_GS2U_LimitSaleInfo:
		data, _ := READ_MSG_GS2U_LimitSaleInfo(bin)

		return data
	case CMD_MSG_U2GS_Get_Limit_Sale_Activity_Info:
		data, _ := READ_MSG_U2GS_Get_Limit_Sale_Activity_Info(bin)

		return data
	case CMD_MSG_GS2U_Get_Limit_Sale_Activity_Info_Result:
		data, _ := READ_MSG_GS2U_Get_Limit_Sale_Activity_Info_Result(bin)

		return data
	case CMD_MSG_U2GS_LimitSaleActivity_Buy_Item:
		data, _ := READ_MSG_U2GS_LimitSaleActivity_Buy_Item(bin)

		return data
	case CMD_MSG_GS2U_LimitSaleActivity_Buy_Item_Result:
		data, _ := READ_MSG_GS2U_LimitSaleActivity_Buy_Item_Result(bin)

		return data
	case CMD_MSG_LodePlayerInfo:
		data, _ := READ_MSG_LodePlayerInfo(bin)

		return data
	case CMD_MSG_LodePlayerHeroInfo:
		data, _ := READ_MSG_LodePlayerHeroInfo(bin)

		return data
	case CMD_MSG_LodePlayerInfo_s:
		data, _ := READ_MSG_LodePlayerInfo_s(bin)

		return data
	case CMD_MSG_LodePositionBaseInfo:
		data, _ := READ_MSG_LodePositionBaseInfo(bin)

		return data
	case CMD_MSG_U2GS_RequestLodeInfo:
		data, _ := READ_MSG_U2GS_RequestLodeInfo(bin)

		return data
	case CMD_MSG_GS2U_ResponseLodeInfo:
		data, _ := READ_MSG_GS2U_ResponseLodeInfo(bin)

		return data
	case CMD_MSG_U2GS_RequestLodeArea:
		data, _ := READ_MSG_U2GS_RequestLodeArea(bin)

		return data
	case CMD_MSG_GS2U_ResponseLodeAreaInfo:
		data, _ := READ_MSG_GS2U_ResponseLodeAreaInfo(bin)

		return data
	case CMD_MSG_U2GS_RequestLodeLoactionInfo:
		data, _ := READ_MSG_U2GS_RequestLodeLoactionInfo(bin)

		return data
	case CMD_MSG_GS2U_ResponseLodeLocationInfo:
		data, _ := READ_MSG_GS2U_ResponseLodeLocationInfo(bin)

		return data
	case CMD_MSG_LodeHistory:
		data, _ := READ_MSG_LodeHistory(bin)

		return data
	case CMD_MSG_U2GS_RequestLodeHistory:
		data, _ := READ_MSG_U2GS_RequestLodeHistory(bin)

		return data
	case CMD_MSG_GS2U_ReponseLodeHistoryList:
		data, _ := READ_MSG_GS2U_ReponseLodeHistoryList(bin)

		return data
	case CMD_MSG_U2GS_RequestLodeFight:
		data, _ := READ_MSG_U2GS_RequestLodeFight(bin)

		return data
	case CMD_MSG_GS2U_ResponseLodeFightResult:
		data, _ := READ_MSG_GS2U_ResponseLodeFightResult(bin)

		return data
	case CMD_MSG_LodeFightInfo:
		data, _ := READ_MSG_LodeFightInfo(bin)

		return data
	case CMD_MSG_U2GS_CollectLodePoint:
		data, _ := READ_MSG_U2GS_CollectLodePoint(bin)

		return data
	case CMD_MSG_GS2U_CollectLodePointResult:
		data, _ := READ_MSG_GS2U_CollectLodePointResult(bin)

		return data
	case CMD_MSG_LodeAwardRecord:
		data, _ := READ_MSG_LodeAwardRecord(bin)

		return data
	case CMD_MSG_U2GS_RequestLodeAwardList:
		data, _ := READ_MSG_U2GS_RequestLodeAwardList(bin)

		return data
	case CMD_MSG_GS2U_ResponseLodeAwardList:
		data, _ := READ_MSG_GS2U_ResponseLodeAwardList(bin)

		return data
	case CMD_MSG_GS2U_PrisonInfo:
		data, _ := READ_MSG_GS2U_PrisonInfo(bin)

		return data
	case CMD_MSG_U2GS_Get_PlayerPrison_Info:
		data, _ := READ_MSG_U2GS_Get_PlayerPrison_Info(bin)

		return data
	case CMD_MSG_GS2U_Get_PlayerPrison_Result:
		data, _ := READ_MSG_GS2U_Get_PlayerPrison_Result(bin)

		return data
	case CMD_MSG_U2GS_PlayerPrison_Refresh:
		data, _ := READ_MSG_U2GS_PlayerPrison_Refresh(bin)

		return data
	case CMD_MSG_GS2U_PlayerPrison_Refresh_Result:
		data, _ := READ_MSG_GS2U_PlayerPrison_Refresh_Result(bin)

		return data
	case CMD_MSG_U2GS_LayerChallengeCampaign:
		data, _ := READ_MSG_U2GS_LayerChallengeCampaign(bin)

		return data
	case CMD_MSG_GS2U_LayerChallengeCampaign_Result:
		data, _ := READ_MSG_GS2U_LayerChallengeCampaign_Result(bin)

		return data
	case CMD_MSG_U2GS_PrisonLayer_Reportserver:
		data, _ := READ_MSG_U2GS_PrisonLayer_Reportserver(bin)

		return data
	case CMD_MSG_GS2U_PrisonLayer_Reportserver_Result:
		data, _ := READ_MSG_GS2U_PrisonLayer_Reportserver_Result(bin)

		return data
	case CMD_MSG_U2GS_Get_Prison_Award:
		data, _ := READ_MSG_U2GS_Get_Prison_Award(bin)

		return data
	case CMD_MSG_GS2U_Get_Prison_Award_Result:
		data, _ := READ_MSG_GS2U_Get_Prison_Award_Result(bin)

		return data
	case CMD_MSG_U2GS_Reset_PrisonLayer:
		data, _ := READ_MSG_U2GS_Reset_PrisonLayer(bin)

		return data
	case CMD_MSG_GS2U_Reset_PrisonLayer_Result:
		data, _ := READ_MSG_GS2U_Reset_PrisonLayer_Result(bin)

		return data
	case CMD_MSG_U2GS_Share_Quest:
		data, _ := READ_MSG_U2GS_Share_Quest(bin)

		return data
	case CMD_MSG_GS2U_Share_Quest_Result:
		data, _ := READ_MSG_GS2U_Share_Quest_Result(bin)

		return data
	case CMD_MSG_U2GS_Player_Explore_Knowledge:
		data, _ := READ_MSG_U2GS_Player_Explore_Knowledge(bin)

		return data
	case CMD_MSG_GS2U_Player_Explore_Knowledge_Result:
		data, _ := READ_MSG_GS2U_Player_Explore_Knowledge_Result(bin)

		return data
	case CMD_MSG_GS2U_KillRobberInfo:
		data, _ := READ_MSG_GS2U_KillRobberInfo(bin)

		return data
	case CMD_MSG_GS2U_KillRobberChapterInfo:
		data, _ := READ_MSG_GS2U_KillRobberChapterInfo(bin)

		return data
	case CMD_MSG_GS2U_Player_KillRobber_List:
		data, _ := READ_MSG_GS2U_Player_KillRobber_List(bin)

		return data
	case CMD_MSG_U2GS_Get_KillRobberChapterAward:
		data, _ := READ_MSG_U2GS_Get_KillRobberChapterAward(bin)

		return data
	case CMD_MSG_GS2U_Get_KillRobberChapterAward_Result:
		data, _ := READ_MSG_GS2U_Get_KillRobberChapterAward_Result(bin)

		return data
	case CMD_MSG_U2GS_Begin_KillRobber:
		data, _ := READ_MSG_U2GS_Begin_KillRobber(bin)

		return data
	case CMD_MSG_KillRobber_Crit:
		data, _ := READ_MSG_KillRobber_Crit(bin)

		return data
	case CMD_MSG_GS2U_Begin_KillRobber_Result:
		data, _ := READ_MSG_GS2U_Begin_KillRobber_Result(bin)

		return data
	case CMD_MSG_U2GS_Sub_KillRobber:
		data, _ := READ_MSG_U2GS_Sub_KillRobber(bin)

		return data
	case CMD_MSG_GS2U_Sub_KillRobber_Result:
		data, _ := READ_MSG_GS2U_Sub_KillRobber_Result(bin)

		return data
	case CMD_MSG_GS2U_End_KillRobber_Result:
		data, _ := READ_MSG_GS2U_End_KillRobber_Result(bin)

		return data
	case CMD_MSG_U2GS_IntoDareKillRobber:
		data, _ := READ_MSG_U2GS_IntoDareKillRobber(bin)

		return data
	case CMD_MSG_GS2U_IntoDareKillRobber_Result:
		data, _ := READ_MSG_GS2U_IntoDareKillRobber_Result(bin)

		return data
	case CMD_MSG_U2GS_DareKillRobber_Reportserver:
		data, _ := READ_MSG_U2GS_DareKillRobber_Reportserver(bin)

		return data
	case CMD_MSG_GS2U_DareKillRobber_Reportserver_Result:
		data, _ := READ_MSG_GS2U_DareKillRobber_Reportserver_Result(bin)

		return data
	case CMD_MSG_U2GS_Equip_Random_Compose:
		data, _ := READ_MSG_U2GS_Equip_Random_Compose(bin)

		return data
	case CMD_MSG_GS2U_Equip_Random_Compose_Result:
		data, _ := READ_MSG_GS2U_Equip_Random_Compose_Result(bin)

		return data
	case CMD_MSG_GS2U_CommonData:
		data, _ := READ_MSG_GS2U_CommonData(bin)
		return data

	default:
		fmt.Printf("unknown cmd:%d", cmd)
	}
	return nil
}

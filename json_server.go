package main

import (
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"sort"

	"github.com/gin-gonic/gin"

	// "fmt"
	"os"
	"strconv"

	_ "github.com/lib/pq"
)

type DataBaseInformation struct {
	UserName       string `json:"UserName"`
	Password       string `json:"Password"`
	RawEventsTable string `json:"RawEventsTable"`
	RawFilesTable  string `json:"RawFilesTable"`
}

var dbInfo DataBaseInformation

func main() {

	bytes, err := os.ReadFile("json_server_conf.json")
	if err != nil {
		log.Fatal(err)
	}
	// JSONデコード

	if err := json.Unmarshal(bytes, &dbInfo); err != nil {
		log.Fatal(err)
	}
	// デコードしたデータを表示
	fmt.Printf("UserName : %s\n", dbInfo.UserName)
	fmt.Printf("Password : %s\n", dbInfo.Password)
	fmt.Printf("RawEventsTable : %s\n", dbInfo.RawEventsTable)
	fmt.Printf("RawFilesTable : %s\n", dbInfo.RawFilesTable)

	r := gin.Default()
	// r.GET("/ping", func(c *gin.Context) {
	// 	c.JSON(200, gin.H{
	// 		"message": "pong",
	// 	})
	// })
	r.GET("/get/:run_id/:event_number", select_test)
	r.Run() // 0.0.0.0:8080 でサーバーを立てます。
}

type EventIndex struct {
	RunID                uint32 `json:"RunID"`
	PlaneID              uint32 `json:"PlaneID"`
	BoardID              uint32 `json:"BoardID"`
	FilePath             string `json:"FilePath"`
	EventDataAddress     uint64 `json:"EventDataAddress"`
	EventDataLength      uint32 `json:"EventDataLength"`
	EventFADCWordsOffset uint32 `json:"EventFADCWordsOffset"`
	EventTPCWordsOffset  uint32 `json:"EventTPCWordsOffset"`
}

func select_test(c *gin.Context) {
	// db, err := sql.Open("postgres", "host=postgres port=5432 user=quser dbmane=quser password=rcnpdaq sslmode=disable")
	postgresOption := "user=" + dbInfo.UserName + " " + "password=" + dbInfo.Password
	db, err := sql.Open("postgres", postgresOption)

	run_id, _ := strconv.Atoi(c.Param("run_id"))
	event_number, _ := strconv.ParseInt(c.Param("event_number"), 10, 64)
	defer db.Close()

	if err != nil {
		// fmt.Println(err)
		c.JSON(500, gin.H{"Error": err})
		return
	}

	select_query := "SELECT " +
		"e.run_id, e.plane_id, e.board_id, f.file_path, " +
		"e.event_data_address, e.event_data_length, " +
		"e.event_fadc_words_offset, e.event_tpc_words_offset " +
		"FROM " + dbInfo.RawEventsTable + " AS e " +
		"INNER JOIN " + dbInfo.RawFilesTable + " AS f ON " +
		"e.run_id = f.run_id AND " +
		"e.plane_id = f.plane_id AND " +
		"e.board_id = f.board_id AND " +
		"e.file_number = f.file_number " +
		"WHERE " +
		"e.run_id = " + strconv.Itoa(run_id) + " AND " +
		"e.event_trigger_counter = " + strconv.FormatInt(event_number, 10) + " " +
		"ORDER BY (e.plane_id, e.board_id)"

	// fmt.Println(select_query)
	// rows, err := db.Query("SELECT run_id FROM test.raw_events WHERE event_trigger_counter = 1 AND board_id = 0")
	rows, err := db.Query(select_query)

	if err != nil {
		c.JSON(500, gin.H{"Error": err})
		return
	}

	// fmt.Println(rows)
	var indexes []EventIndex
	for rows.Next() {
		var ind EventIndex
		rows.Scan(&ind.RunID, &ind.PlaneID, &ind.BoardID, &ind.FilePath,
			&ind.EventDataAddress, &ind.EventDataLength, &ind.EventFADCWordsOffset, &ind.EventTPCWordsOffset)
		indexes = append(indexes, ind)
		fmt.Println(ind)
	}

	chResults := make(chan DecodeRawFileResult, len(indexes))
	var results []DecodeRawFileResult

	var evt BuiltEventData
	if len(indexes) > 0 {
		for _, index := range indexes {
			go func(ind EventIndex, event_number uint64, chResult chan DecodeRawFileResult) {
				frg, good := DecodeRawFile(ind, uint64(event_number))
				var result DecodeRawFileResult
				result.Fragment = frg
				result.GoodFlag = good
				chResult <- result
			}(index, uint64(event_number), chResults)
			fmt.Println(len(indexes))
		}
		// DecodeRawFile(indexes[1], uint64(event_number))

		for i := 0; i < len(indexes); i++ {
			result := <-chResults
			results = append(results, result)
			if result.GoodFlag {
				evt.AddFragment(result.Fragment)
			} else {
				fmt.Println("ill decode")
				fmt.Println(result.Fragment.PlaneID)
				fmt.Println(result.Fragment.BoardID)
				fmt.Println(len(result.Fragment.FADCData))
			}
			fmt.Println(len(results))
		}
	} else {
		c.JSON(500, gin.H{"Error": "No such event."})
		return
	}

	// c.JSON(200, gin.H{"OK": "ok"})
	// c.JSON(200, gin.H{"Indexes": indexes})

	// c.JSON(200, results)
	var ret APIFormat
	ret.AnodeHit = EncodeHitsIntoTOTArray(evt.GetHits(0))
	ret.CathodeHit = EncodeHitsIntoTOTArray(evt.GetHits(1))
	for iCh := uint32(0); iCh < 24; iCh++ {
		ret.AnodeFADC = append(ret.AnodeFADC, evt.GetSignal(0, iCh))
		ret.CathodeFADC = append(ret.CathodeFADC, evt.GetSignal(1, iCh))
	}
	c.JSON(200, ret)
	return

}

type CounterData struct {
	TriggerCounter uint32 `json:"TriggerCounter"`
	ClockCounter   uint32 `json:"ClockCounter"`
	ScalerCounter  uint32 `json:"ScalerCounter"`
}

type Hit struct {
	Strip uint32
	Clock uint32
}

type FragmentedEventData struct {
	RunID       uint32
	PlaneID     uint32
	BoardID     uint32
	EventNumber uint64
	Counter     CounterData
	FADCData    [][]uint16
	TPCData     []Hit
}

type DecodeRawFileResult struct {
	Fragment FragmentedEventData
	GoodFlag bool
}

type BuiltEventData struct {
	fragments []FragmentedEventData
}

func (bd *BuiltEventData) AddFragment(frgIn FragmentedEventData) bool {
	duplication := false
	for _, frg := range bd.fragments {
		if frg.PlaneID == frgIn.PlaneID &&
			frg.BoardID == frgIn.BoardID {
			duplication = true
		}
	}
	if !duplication {
		bd.fragments = append(bd.fragments, frgIn)
	}
	return !(duplication)
}

func (bd *BuiltEventData) GetHits(planeID uint32) []Hit {
	var hits []Hit
	if planeID != 0 && planeID != 1 {
		return hits
	}

	for _, frg := range bd.fragments {
		pi := frg.PlaneID
		if pi != planeID {
			continue
		}
		brd := frg.BoardID
		for _, hit := range frg.TPCData {
			var hitAdd Hit
			// Mapping
			strip := hit.Strip + brd*128
			clock := hit.Clock
			hitAdd.Strip = strip
			hitAdd.Clock = clock
			hits = append(hits, hitAdd)
		}

	}
	return hits
}

func (bd *BuiltEventData) GetSignal(planeID uint32, channel uint32) []uint16 {
	var signal []uint16

	boardID := channel / 4
	chInBoard := channel % 4

	for _, frg := range bd.fragments {
		pi := frg.PlaneID
		brd := frg.BoardID

		if pi == planeID && brd == boardID && len(frg.FADCData) > int(chInBoard) {
			signal = frg.FADCData[chInBoard]
		}
	}

	return signal
}

func EncodeHitsIntoArray(hits []Hit) [][]uint32 {
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Strip != hits[j].Strip {
			return hits[i].Strip < hits[j].Strip
		} else {
			return hits[i].Clock < hits[j].Clock
		}
	})

	var arr [][]uint32
	for _, hit := range hits {
		ha := make([]uint32, 2)
		ha[0] = hit.Strip
		ha[1] = hit.Clock
		arr = append(arr, ha)
	}

	return arr
}

// [0]: strip, [1]: clock, [2]: TOT
// TOT >= 1
// Signal begins at t = clock and continues till t = clock + TOT - "1"
func EncodeHitsIntoTOTArray(hits []Hit) [][]uint32 {
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Strip != hits[j].Strip {
			return hits[i].Strip < hits[j].Strip
		} else {
			return hits[i].Clock < hits[j].Clock
		}
	})

	var arr [][]uint32

	if len(hits) > 0 {
		i := 0
		for {

			if i >= len(hits) {
				break
			}

			strip := hits[i].Strip
			clock := hits[i].Clock

			j := i + 1
			for {
				if j < len(hits) &&
					hits[j].Strip == strip &&
					hits[j].Clock == ((clock)+uint32(j-i)) {
					j++
				} else {
					j--
					break
				}
			}
			tot := uint32(j-i) + 1
			ha := make([]uint32, 3)
			ha[0] = strip
			ha[1] = clock
			ha[2] = tot
			arr = append(arr, ha)
			i = j + 1

		}
	}

	return arr
}

type APIFormat struct {
	AnodeHit    [][]uint32
	CathodeHit  [][]uint32
	AnodeFADC   [][]uint16
	CathodeFADC [][]uint16
}

func DecodeRawFile(ind EventIndex, eventNumber uint64) (FragmentedEventData, bool) {

	var data FragmentedEventData
	data.RunID = ind.RunID
	data.PlaneID = ind.PlaneID
	data.BoardID = ind.BoardID
	data.EventNumber = eventNumber

	file, err := os.Open(ind.FilePath)

	defer file.Close()
	if err != nil {
		return data, false
	}

	file.Seek(int64(ind.EventDataAddress), 0)

	wordsEvent := make([]uint32, ind.EventDataLength/(32/8))

	errb := binary.Read(file, binary.BigEndian, &wordsEvent)

	if errb != nil {
		return data, false
	}

	// fmt.Printf("%#x\n", wordsEvent[0])
	// fmt.Printf("%#x\n", wordsEvent[len(wordsEvent)-1])

	if len(wordsEvent) < 4 ||
		wordsEvent[0] != 0xeb901964 ||
		wordsEvent[len(wordsEvent)-1] != 0x75504943 {
		return data, false
	}

	var counterData []uint32
	var fadcData []uint32
	var tpcData []uint32

	counterData = append(counterData, wordsEvent[1:4]...)
	data.Counter.TriggerCounter = counterData[0]
	data.Counter.ClockCounter = counterData[1]
	data.Counter.ScalerCounter = counterData[2]

	if ind.EventFADCWordsOffset != 0 && ind.EventTPCWordsOffset != 0 {
		fadcData = append(fadcData, wordsEvent[ind.EventFADCWordsOffset:ind.EventTPCWordsOffset-1]...)
		tpcData = append(tpcData, wordsEvent[ind.EventTPCWordsOffset:len(wordsEvent)-1]...)
	}
	fmt.Println("length == ", len(wordsEvent), len(fadcData), len(tpcData))
	signals := make([][]uint16, 4)
	if len(fadcData) > 0 && len(fadcData)%2 == 0 {
		for i := 0; i < len(fadcData); i = i + 2 {
			word1 := fadcData[i]
			word2 := fadcData[i+1]

			sWord0 := uint16((word1 & 0xffff0000) >> 16)
			sWord1 := uint16((word1 & 0x0000ffff))
			sWord2 := uint16((word2 & 0xffff0000) >> 16)
			sWord3 := uint16((word2 & 0x0000ffff))

			if (sWord0&0xc000) == 0x4000 && ((sWord0&0x3000)>>12) == 0 &&
				(sWord1&0xc000) == 0x4000 && ((sWord1&0x3000)>>12) == 1 &&
				(sWord2&0xc000) == 0x4000 && ((sWord2&0x3000)>>12) == 2 &&
				(sWord3&0xc000) == 0x4000 && ((sWord3&0x3000)>>12) == 3 {
				signals[0] = append(signals[0], sWord0&0x3ff)
				signals[1] = append(signals[1], sWord1&0x3ff)
				signals[2] = append(signals[2], sWord2&0x3ff)
				signals[3] = append(signals[3], sWord3&0x3ff)
			}
			// } else {
			// 	fmt.Println(sWord0)
			// }
		}
	}

	var hits []Hit
	if len(tpcData) > 0 && len(tpcData)%5 == 0 {
		for i := 0; i < len(tpcData); i = i + 5 {

			var headerWord uint32
			headerWord = tpcData[i]
			var words [4]uint32
			words[0] = tpcData[i+1]
			words[1] = tpcData[i+2]
			words[2] = tpcData[i+3]
			words[3] = tpcData[i+4]

			if headerWord&0xffff0000 == 0x80000000 {
				var clock uint32
				clock = headerWord & 0x0000ffff

				nBitsWord := 32
				for iWord, word := range words {
					stripShift := (len(words) - 1 - int(iWord)) * nBitsWord
					for iBit := 0; iBit < nBitsWord; iBit++ {
						stripInner := iBit
						if (word&(0x00000001<<iBit))>>iBit == 1 {
							// fmt.Println(stripInner+stripShift, " ", clock)
							var hit Hit
							hit.Clock = clock
							hit.Strip = uint32(stripInner) + uint32(stripShift)
							hits = append(hits, hit)
						}
					}
				}

			}

		}
	} else {
		// fmt.Println("No TPC ", len(tpcData), " ", ind.EventTPCWordsOffset, " ", len(wordsEvent)-2)
	}

	data.FADCData = signals
	data.TPCData = hits

	// if len(data.FADCData[0]) != len(fadcData)*2 {
	// 	return data, false
	// }

	return data, true
}

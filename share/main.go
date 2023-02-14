package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"strconv"

	"github.com/bnb-chain/tss-lib/ecdsa/keygen"
	"github.com/bnb-chain/tss-lib/tss"
	"github.com/pkg/errors"
	amqp "github.com/rabbitmq/amqp091-go"
)

func failOnError(err error, msg string) {
	if err != nil {
		fmt.Printf("%s: %s\n", msg, err)
	}
}
func main() {
	msg := &big.Int{}
	data, _ := hex.DecodeString("c114d5dd701a9904226fe3553175f68e0f777a03072dacba8922dd960661eb39")
	msg.SetBytes([]byte(data))

	if len(os.Args) < 2 {
		fmt.Println(nil, "Input party number and is new party")
	}
	arg := os.Args[1]
	partyInt, _ := strconv.Atoi(arg)
	arg2 := os.Args[2]
	isNewParty := arg2 == "true"

	conn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
	failOnError(err, "Failed to connect to RabbitMQ")
	defer conn.Close()

	NewSharing(partyInt, 3, 4, 5, 3, isNewParty, conn)
}

func LoadData(qty int) ([]keygen.LocalPartySaveData, tss.SortedPartyIDs, error) {
	keys := make([]keygen.LocalPartySaveData, 0, qty)
	// plucked := make(map[int]interface{}, qty)
	// for i := 0; len(plucked) < qty; i = (i + 1) {
	// 	_, have := plucked[i]
	// 	if pluck := 0 < 0.5; !have && pluck {
	// 		plucked[i] = new(struct{})
	// 	}
	// }
	partyIDs := make(tss.UnSortedPartyIDs, qty)
	for i := 0; i < qty; i++ {
		fixtureFilePath := fmt.Sprintf("keygen%d", i)
		bz, err := ioutil.ReadFile(fixtureFilePath)
		if err != nil {
			return nil, nil, errors.Wrapf(err,
				"could not open the test fixture for party %d in the expected location: %s. run keygen tests first.",
				i, fixtureFilePath)
		}
		var key keygen.LocalPartySaveData
		if err = json.Unmarshal(bz, &key); err != nil {
			return nil, nil, errors.Wrapf(err,
				"could not unmarshal fixture data for party %d located at: %s",
				i, fixtureFilePath)
		}
		pMoniker := fmt.Sprintf("%d", i+1)
		partyIDs[i] = tss.NewPartyID(pMoniker, pMoniker, key.ShareID)
		keys = append(keys, key)
	}
	// j := 0
	// for i := range plucked {
	// 	key := keys[j]
	// 	pMoniker := fmt.Sprintf("%d", i+1)
	// 	partyIDs[j] = tss.NewPartyID(pMoniker, pMoniker, key.ShareID)
	// 	j++
	// }
	sortedPIDs := tss.SortPartyIDs(partyIDs)
	// sort.Slice(keys, func(i, j int) bool { return keys[i].ShareID.Cmp(keys[j].ShareID) == -1 })
	return keys, sortedPIDs, nil
}

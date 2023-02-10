package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"runtime"
	"strconv"

	"github.com/bnb-chain/tss-lib/common"
	. "github.com/bnb-chain/tss-lib/ecdsa/signing"
	"github.com/bnb-chain/tss-lib/tss"
	amqp "github.com/rabbitmq/amqp091-go"
)

func NewSigning(partyInt int, message big.Int, threshold int, numParties int, conn *amqp.Connection) {

	pIDs := GenerateTestPartyIDs(numParties)
	keys, sigPIDs, err := LoadData(numParties)
	if err != nil {
		fmt.Printf("err: %v\n", err)
	}
	fmt.Println(sigPIDs)
	p2pCtx := tss.NewPeerContext(pIDs)

	errCh := make(chan *tss.Error, len(pIDs))
	outCh := make(chan tss.Message, len(pIDs))
	endCh := make(chan common.SignatureData, len(pIDs))

	// updater := test.SharedPartyUpdater

	// init the parties
	var P *LocalParty
	params := tss.NewParameters(tss.S256(), p2pCtx, pIDs[partyInt-1], len(pIDs), threshold)
	P = NewLocalParty(&message, params, keys[partyInt-1], outCh, endCh).(*LocalParty)
	ch, err := conn.Channel()
	failOnError(err, "Failed to open a channel")
	defer ch.Close()
	go func(P *LocalParty) {
		if err := P.Start(); err != nil {
			errCh <- err
		}
	}(P)
signing:
	for {
		fmt.Printf("ACTIVE GOROUTINES: %d\n", runtime.NumGoroutine())
		select {
		case err := <-errCh:
			common.Logger.Errorf("Error: %s", err)
			break signing

		case msg := <-outCh:
			dest := msg.GetTo()
			messages := ReciveMessage(ch, strconv.Itoa(msg.GetFrom().Index))
			go func() {
				for ms := range messages {
					msgBytes := ms.Body

					msg := msgBytes[:len(msgBytes)-2]
					fromByte := msgBytes[len(msgBytes)-2 : len(msgBytes)-1]
					fromIndex, _ := strconv.Atoi(string(fromByte))
					broatcastByte := msgBytes[len(msgBytes)-1:]
					broatcast, _ := strconv.Atoi(string(broatcastByte))

					isBroatcast := false
					if broatcast == 1 {
						isBroatcast = true
					}

					P.UpdateFromBytes(msg, pIDs[fromIndex], isBroatcast)
				}
			}()
			if dest == nil { // broadcast!
				for _, P := range pIDs {
					if P.Index == msg.GetFrom().Index {
						continue
					}
					// go updater(P, msg, errCh)
					msgBytes, _, err := msg.WireBytes()
					failOnError(err, "Failed to convert WireBytes")
					broadcast := 0
					if msg.IsBroadcast() {
						broadcast = 1
					}
					SendMessage(ch, strconv.Itoa(P.Index), append(append(msgBytes, []byte(strconv.Itoa(msg.GetFrom().Index))...), []byte(strconv.Itoa(broadcast))...))
				}
			} else { // point-to-point!
				if dest[0].Index == msg.GetFrom().Index {
					fmt.Printf("party %d tried to send a message to itself (%d)", dest[0].Index, msg.GetFrom().Index)
					return
				}
				// go updater(parties[dest[0].Index], msg, errCh)
				msgBytes, _, err := msg.WireBytes()
				failOnError(err, "Failed to convert WireBytes")
				broadcast := 0
				if msg.IsBroadcast() {
					broadcast = 1
				}
				SendMessage(ch, strconv.Itoa(dest[0].Index), append(append(msgBytes, []byte(strconv.Itoa(msg.GetFrom().Index))...), []byte(strconv.Itoa(broadcast))...))
			}

		case save := <-endCh:
			// SAVE a test fixture file for this P (if it doesn't already exist)
			// .. here comes a workaround to recover this party's index (it was removed from save data)
			StorageSavedata(&save, fmt.Sprintf("signature.txt"))
			break signing
		}
	}
}

func GenerateTestPartyIDs(count int, startAt ...int) tss.SortedPartyIDs {
	ids := make(tss.UnSortedPartyIDs, 0, count)
	// key := common.MustGetRandomInt(256)
	frm := 0
	i := 0 // default `i`
	if len(startAt) > 0 {
		frm = startAt[0]
		i = startAt[0]
	}
	for ; i < count+frm; i++ {
		ids = append(ids, &tss.PartyID{
			MessageWrapper_PartyID: &tss.MessageWrapper_PartyID{
				Id:      fmt.Sprintf("%d", i+1),
				Moniker: fmt.Sprintf("P[%d]", i+1),
				Key:     []byte(strconv.Itoa(i + 1)),
			},
			Index: i,
			// this key makes tests more deterministic
		})
	}
	return tss.SortPartyIDs(ids, startAt...)
}

func StorageSavedata(sv *common.SignatureData, path string) error {
	perm := os.FileMode(0600)

	b, err := json.Marshal(sv)
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile(path, b, perm); err != nil {
		return err
	}
	return nil
}

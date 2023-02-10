package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/bnb-chain/tss-lib/common"
	"github.com/bnb-chain/tss-lib/ecdsa/keygen"
	. "github.com/bnb-chain/tss-lib/ecdsa/resharing"
	"github.com/bnb-chain/tss-lib/tss"
	amqp "github.com/rabbitmq/amqp091-go"
)

func NewSharing(partyInt int, threshold int, numParties int, newParties int, newThreshold int, conn *amqp.Connection) {

	newPIDs := GenerateTestPartyIDs(newParties)
	newP2PCtx := tss.NewPeerContext(newPIDs)
	newPCount := len(newPIDs)

	oldKeys, oldPIDs, err := LoadData(numParties)
	if err != nil {
		fmt.Printf("err: %v\n", err)
	}
	oldP2PCtx := tss.NewPeerContext(oldPIDs)
	bothCommitteesPax := numParties + newParties

	errCh := make(chan *tss.Error, bothCommitteesPax)
	outCh := make(chan tss.Message, bothCommitteesPax)
	endCh := make(chan keygen.LocalPartySaveData, bothCommitteesPax)

	var oldP *LocalParty
	var newP *LocalParty

	newParams := tss.NewReSharingParameters(tss.S256(), oldP2PCtx, newP2PCtx, newPIDs[partyInt-1], numParties, threshold, newParties, newThreshold)
	preParams, _ := keygen.GeneratePreParams(1 * time.Minute)
	save := keygen.NewLocalPartySaveData(newPCount)
	save.LocalPreParams = *preParams
	newP = NewLocalParty(newParams, save, outCh, endCh).(*LocalParty)
	fmt.Printf("newP.PartyID().Index: %v\n", newP.PartyID().Index)
	ch, err := conn.Channel()
	failOnError(err, "Failed to open a channel")
	defer ch.Close()
	go func(P *LocalParty) {
		if err := P.Start(); err != nil {
			errCh <- err
		}
		if partyInt > numParties && numParties < newParties {
			messages := ReciveMessage(ch, strconv.Itoa(P.PartyID().Index))
			fmt.Printf("dest: %v\n", P.PartyID().Index)
			go func() {
				for ms := range messages {
					msgBytes := ms.Body

					msg := msgBytes[:len(msgBytes)-3]
					fromByte := msgBytes[len(msgBytes)-3 : len(msgBytes)-2]
					fromIndex, _ := strconv.Atoi(string(fromByte))
					broatcastByte := msgBytes[len(msgBytes)-2 : len(msgBytes)-1]
					broatcast, _ := strconv.Atoi(string(broatcastByte))
					oldBytes := msgBytes[len(msgBytes)-1]
					old, _ := strconv.Atoi(string(oldBytes))

					isBroatcast := false
					if broatcast == 1 {
						isBroatcast = true
					}

					if old == 1 {
						oldP.UpdateFromBytes(msg, oldPIDs[fromIndex], isBroatcast)
					} else {
						newP.UpdateFromBytes(msg, newPIDs[fromIndex], isBroatcast)
					}
				}
			}()
		}
	}(newP)
	if partyInt <= len(oldPIDs) {
		params := tss.NewReSharingParameters(tss.S256(), oldP2PCtx, newP2PCtx, oldPIDs[partyInt-1], numParties, threshold, newParties, newThreshold)
		oldP = NewLocalParty(params, oldKeys[partyInt-1], outCh, endCh).(*LocalParty) // discard old key data
		go func(P *LocalParty) {
			if err := P.Start(); err != nil {
				errCh <- err
			}
		}(oldP)
	}
	endedOldCommittee := 0

	for {
		fmt.Printf("ACTIVE GOROUTINES: %d\n", runtime.NumGoroutine())

		select {
		case err := <-errCh:
			common.Logger.Errorf("Error: %s", err)
			return

		case msg := <-outCh:
			dest := msg.GetTo()
			messages := ReciveMessage(ch, strconv.Itoa(msg.GetFrom().Index))
			fmt.Printf("dest: %v\n", dest)
			go func() {
				for ms := range messages {
					msgBytes := ms.Body

					msg := msgBytes[:len(msgBytes)-3]
					fromByte := msgBytes[len(msgBytes)-3 : len(msgBytes)-2]
					fromIndex, _ := strconv.Atoi(string(fromByte))
					broatcastByte := msgBytes[len(msgBytes)-2 : len(msgBytes)-1]
					broatcast, _ := strconv.Atoi(string(broatcastByte))
					oldBytes := msgBytes[len(msgBytes)-1]
					old, _ := strconv.Atoi(string(oldBytes))
					fmt.Printf("old: %v\n", old)
					isBroatcast := false
					if broatcast == 1 {
						isBroatcast = true
					}

					if old == 1 && numParties > fromIndex {
						oldP.UpdateFromBytes(msg, oldPIDs[fromIndex], isBroatcast)
					} else {
						newP.UpdateFromBytes(msg, newPIDs[fromIndex], isBroatcast)
					}
				}
			}()
			if dest == nil {
				fmt.Println("did not expect a msg to have a nil destination during resharing")
				return
			}
			if msg.IsToOldCommittee() || msg.IsToOldAndNewCommittees() {
				for _, destP := range dest[:len(oldPIDs)] {
					// go updater(oldCommittee[destP.Index], msg, errCh)
					msgBytes, _, _ := msg.WireBytes()
					oldBytes := []byte("1")
					from := []byte(strconv.Itoa(msg.GetFrom().Index))
					broadcast := "0"
					if msg.IsBroadcast() {
						broadcast = "1"
					}
					broadcastBytes := []byte(broadcast)
					SendMessage(ch, strconv.Itoa(destP.Index), append((append(append(msgBytes, from...), broadcastBytes...)), oldBytes...))
				}
			}

			if !msg.IsToOldCommittee() || msg.IsToOldAndNewCommittees() {
				for _, destP := range dest {

					msgBytes, _, _ := msg.WireBytes()
					oldBytes := []byte("0")
					from := []byte(strconv.Itoa(msg.GetFrom().Index))
					broadcast := "0"
					if msg.IsBroadcast() {
						broadcast = "1"
					}
					broadcastBytes := []byte(broadcast)
					SendMessage(ch, strconv.Itoa(destP.Index), append((append(append(msgBytes, from...), broadcastBytes...)), oldBytes...))
				}
			}
		case save := <-endCh:
			// SAVE a test fixture file for this P (if it doesn't already exist)
			// .. here comes a workaround to recover this party's index (it was removed from save data)
			// StorageSavedata(&save, fmt.Sprintf("signature.txt"))
			// break signing

			if save.Xi != nil {
				index, err := save.OriginalIndex()
				if err != nil {
					fmt.Printf("err: %v\n", err)
				}
				StorageSavedata(&save, fmt.Sprintf("sharing%d.txt", index))
				return
			} else {
				endedOldCommittee++
			}
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
				Id:      fmt.Sprintf("%d", i+10),
				Moniker: fmt.Sprintf("P[%d]", i+10),
				Key:     []byte(strconv.Itoa(i + 10)),
			},
			Index: i,
			// this key makes tests more deterministic
		})
	}
	return tss.SortPartyIDs(ids, startAt...)
}

func StorageSavedata(sv *keygen.LocalPartySaveData, path string) error {
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

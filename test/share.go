package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/bnb-chain/tss-lib/common"
	"github.com/bnb-chain/tss-lib/ecdsa/keygen"
	. "github.com/bnb-chain/tss-lib/ecdsa/resharing"
	"github.com/bnb-chain/tss-lib/test"
	"github.com/bnb-chain/tss-lib/tss"
)

func main() {
	testThreshold := 10
	testParticipants := 20
	newTestThreshold := 11
	newTestParticipants := 21
	threshold, newThreshold := testThreshold, newTestThreshold

	// PHASE: load keygen fixtures
	firstPartyIdx, extraParties := 5, 1 // extra can be 0 to N-first
	fmt.Println("testThreshold+1+extraParties+firstPartyIdx", testThreshold+1+extraParties+firstPartyIdx)
	oldKeys, oldPIDs, err := keygen.LoadKeygenTestFixtures(testThreshold+1+extraParties+firstPartyIdx, firstPartyIdx)

	// PHASE: resharing
	oldP2PCtx := tss.NewPeerContext(oldPIDs)
	// init the new parties; re-use the fixture pre-params for speed
	// fixtures, _, err := keygen.LoadKeygenTestFixtures(testParticipants)
	if err != nil {
		common.Logger.Info("No test fixtures were found, so the safe primes will be generated from scratch. This may take a while...")
	}
	newPIDs := tss.GenerateTestPartyIDs(newTestParticipants)
	newP2PCtx := tss.NewPeerContext(newPIDs)
	newPCount := len(newPIDs)

	oldCommittee := make([]*LocalParty, 0, len(oldPIDs))
	newCommittee := make([]*LocalParty, 0, newPCount)
	bothCommitteesPax := len(oldCommittee) + len(newCommittee)

	errCh := make(chan *tss.Error, bothCommitteesPax)
	outCh := make(chan tss.Message, bothCommitteesPax)
	endCh := make(chan keygen.LocalPartySaveData, bothCommitteesPax)

	updater := test.SharedPartyUpdater

	// init the old parties first
	for j, pID := range oldPIDs {
		params := tss.NewReSharingParameters(tss.S256(), oldP2PCtx, newP2PCtx, pID, testParticipants, threshold, newPCount, newThreshold)
		P := NewLocalParty(params, oldKeys[j], outCh, endCh).(*LocalParty) // discard old key data
		oldCommittee = append(oldCommittee, P)
	}
	// init the new parties
	for _, pID := range newPIDs {
		params := tss.NewReSharingParameters(tss.S256(), oldP2PCtx, newP2PCtx, pID, testParticipants, threshold, newPCount, newThreshold)
		save := keygen.NewLocalPartySaveData(newPCount)
		// if j < len(fixtures) && len(newPIDs) <= len(fixtures) {
		// 	save.LocalPreParams = fixtures[j].LocalPreParams
		// }
		pre, _ := keygen.GeneratePreParams(time.Minute)
		save.LocalPreParams = *pre
		P := NewLocalParty(params, save, outCh, endCh).(*LocalParty)
		newCommittee = append(newCommittee, P)
	}

	// start the new parties; they will wait for messages
	for _, P := range newCommittee {
		go func(P *LocalParty) {
			if err := P.Start(); err != nil {
				errCh <- err
			}
		}(P)
	}
	// start the old parties; they will send messages
	for _, P := range oldCommittee {
		go func(P *LocalParty) {
			if err := P.Start(); err != nil {
				errCh <- err
			}
		}(P)
	}

	newKeys := make([]keygen.LocalPartySaveData, len(newCommittee))
	endedOldCommittee := 0
	var reSharingEnded int32
	for {
		fmt.Printf("ACTIVE GOROUTINES: %d\n", runtime.NumGoroutine())
		select {
		case err := <-errCh:
			common.Logger.Errorf("Error: %s", err)
			return

		case msg := <-outCh:
			dest := msg.GetTo()
			if dest == nil {
				fmt.Println("did not expect a msg to have a nil destination during resharing")
			}
			if msg.IsToOldCommittee() || msg.IsToOldAndNewCommittees() {
				for _, destP := range dest[:len(oldCommittee)] {
					go updater(oldCommittee[destP.Index], msg, errCh)
				}
			}
			if !msg.IsToOldCommittee() || msg.IsToOldAndNewCommittees() {
				for _, destP := range dest {
					go updater(newCommittee[destP.Index], msg, errCh)
				}
			}

		case save := <-endCh:
			// old committee members that aren't receiving a share have their Xi zeroed
			if save.Xi != nil {
				index, err := save.OriginalIndex()
				if err != nil {
					fmt.Println("should not be an error getting a party's index from save data")
				}
				newKeys[index] = save
			} else {
				endedOldCommittee++
			}
			atomic.AddInt32(&reSharingEnded, 1)
			idx, _ := save.OriginalIndex()
			if idx >= 0 {
				StorageSavedata(&save, fmt.Sprintf("sharing-test%d", idx))
			}

			// if atomic.LoadInt32(&reSharingEnded) == int32(len(oldCommittee)+len(newCommittee)) {
			// 	assert.Equal(t, len(oldCommittee), endedOldCommittee)
			// 	t.Logf("Resharing done. Reshared %d participants", reSharingEnded)

			// }
		}
	}
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

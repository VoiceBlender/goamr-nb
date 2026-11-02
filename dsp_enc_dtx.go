package amrnb

// Encoder side of DTX, ported from opencore-amrnb dtx_enc.cpp (tx_dtx_handler,
// dtx_buffer, dtx_enc) and sid_sync.cpp. With DTX on, the VAD decision drives
// tx_dtx_handler, which switches the coder to MRDTX (comfort noise) after a
// hangover; dtx_enc then quantizes the averaged LSPs and energy of the last
// eight frames into SID parameters, and sid_sync decides which MRDTX frames
// are sent as SID_FIRST, SID_UPDATE or not at all (NO_DATA).

// dtxEncState is the comfort-noise encoder state. Mirrors dtx_encState.
type dtxEncState struct {
	lspHist            [cM * cDTX_HIST_SIZE]int16
	logEnHist          [cDTX_HIST_SIZE]int16
	histPtr            int16
	logEnIndex         int16
	initLsfVqIndex     int16
	lspIndex           [3]int16
	dtxHangoverCount   int16
	decAnaElapsedCount int16
}

// reset mirrors dtx_enc_reset.
func (st *dtxEncState) reset() {
	*st = dtxEncState{}
	for i := 0; i < cDTX_HIST_SIZE; i++ {
		copy(st.lspHist[i*cM:], lsp_init_data[:cM])
	}
	st.dtxHangoverCount = cDTX_HANG_CONST
	st.decAnaElapsedCount = 32767
}

// txDtxHandler returns whether a new SID frame may be computed, and switches
// *usedMode to MRDTX once the VAD has reported no speech for the hangover
// period. Mirrors tx_dtx_handler.
func (st *dtxEncState) txDtxHandler(vadFlag int16, usedMode *int) int16 {
	st.decAnaElapsedCount = add(st.decAnaElapsedCount, 1)
	var computeNewSidPossible int16
	if vadFlag != 0 {
		st.dtxHangoverCount = cDTX_HANG_CONST
	} else if st.dtxHangoverCount == 0 {
		st.decAnaElapsedCount = 0
		*usedMode = dMRDTX
		computeNewSidPossible = 1
	} else {
		st.dtxHangoverCount--
		if add(st.decAnaElapsedCount, st.dtxHangoverCount) < cDTX_ELAPSED_FRAMES_THRESH {
			*usedMode = dMRDTX
		}
	}
	return computeNewSidPossible
}

// buffer records the frame's LSPs and energy. Mirrors dtx_buffer.
func (st *dtxEncState) buffer(lspNew, speech []int16) {
	st.histPtr++
	if st.histPtr == cDTX_HIST_SIZE {
		st.histPtr = 0
	}
	copy(st.lspHist[int(st.histPtr)*cM:], lspNew[:cM])

	var Len int32
	for i := 0; i < cL_FRAME; i++ {
		Len += (int32(speech[i]) * int32(speech[i])) << 1
		if Len < 0 {
			Len = maxInt32
			break
		}
	}
	e, m := log2(Len)
	logEn := shl(e, 10) // e <= 30: no saturation
	logEn += m >> (15 - 10)
	logEn -= 8521
	st.logEnHist[st.histPtr] = logEn >> 1
}

// encode writes the five SID parameters to prm. When a new SID may be
// computed (or none has been yet), it quantizes the history averages and
// resets the gain predictors to the comfort-noise energy. Mirrors dtx_enc.
func (st *dtxEncState) encode(computeSidFlag int16, q *qPlsfState, pred *gcPredState, prm []int16) {
	if computeSidFlag != 0 || st.logEnIndex == 0 {
		var logEn int16
		var Llsp [cM]int32
		for i := cDTX_HIST_SIZE - 1; i >= 0; i-- {
			logEn = add(logEn, st.logEnHist[i]>>2)
			for j := cM - 1; j >= 0; j-- {
				Llsp[j] = L_add(Llsp[j], int32(st.lspHist[i*cM+j]))
			}
		}
		logEn >>= 1
		var lsp, lsf, lspQ [cM]int16
		for j := range lsp {
			lsp[j] = int16(Llsp[j] >> 3)
		}

		st.logEnIndex = logEn + 2560
		st.logEnIndex += 128
		st.logEnIndex >>= 8
		if st.logEnIndex > 63 {
			st.logEnIndex = 63
		} else if st.logEnIndex < 0 {
			st.logEnIndex = 0
		}

		logEn = int16(int32(st.logEnIndex) << (-2 + 10))
		logEn = sub(logEn, 11560)
		if logEn > 0 {
			logEn = 0
		} else if logEn < -14436 {
			logEn = -14436
		}
		for i := range pred.pastQuaEn {
			pred.pastQuaEn[i] = logEn
		}
		logEn = int16((5443 * int32(logEn)) >> 15)
		for i := range pred.pastQuaEnMR122 {
			pred.pastQuaEnMR122[i] = logEn
		}

		lspLsf(lsp[:], lsf[:], cM)
		reorderLsf(lsf[:], cLSF_GAP, cM)
		lsfLsp(lsf[:], lsp[:], cM)
		st.lspIndex, st.initLsfVqIndex = q.qPlsf3DTX(lsp[:], lspQ[:])
	}
	prm[0] = st.initLsfVqIndex
	prm[1] = st.lspIndex[0]
	prm[2] = st.lspIndex[1]
	prm[3] = st.lspIndex[2]
	prm[4] = st.logEnIndex
}

// Transmitted frame types of the encoder (enum TXFrameType).
const (
	txSpeechGood = iota
	txSIDFirst
	txSIDUpdate
	txNoData
)

// sidSyncState schedules SID updates. Mirrors sid_syncState.
type sidSyncState struct {
	updateRate    int16
	updateCounter int16
	handoverDebt  int16
	prevFT        int
}

// reset mirrors sid_sync_init and sid_sync_reset.
func (st *sidSyncState) reset() {
	st.updateRate = 8
	st.updateCounter = 3
	st.handoverDebt = 0
	st.prevFT = txSpeechGood
}

// frameType returns the transmitted frame type for a frame coded in usedMode.
// Mirrors sid_sync.
func (st *sidSyncState) frameType(usedMode int) int {
	var ft int
	if usedMode == dMRDTX {
		st.updateCounter--
		switch {
		case st.prevFT == txSpeechGood:
			ft = txSIDFirst
			st.updateCounter = 3
		case st.handoverDebt > 0 && st.updateCounter > 2:
			ft = txSIDUpdate
			st.handoverDebt--
		case st.updateCounter == 0:
			ft = txSIDUpdate
			st.updateCounter = st.updateRate
		default:
			ft = txNoData
		}
	} else {
		st.updateCounter = st.updateRate
		ft = txSpeechGood
	}
	st.prevFT = ft
	return ft
}

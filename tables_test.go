package amrnb

import "testing"

// TestTableSizes guards the generated ROM tables against silent truncation or
// duplication: every table's length must match the opencore-amrnb C array size
// it was extracted from. (The extractor verifies this at generation time; this
// test makes it a committed regression gate.)
func TestTableSizes(t *testing.T) {
	cases := []struct {
		name string
		got  int
		want int
	}{
		// LSF/LSP quantizer (q_plsf_3_tbl.cpp / q_plsf_5_tbl.cpp)
		{"past_rq_init", len(past_rq_init), 80},
		{"mean_lsf_3", len(mean_lsf_3), 10},
		{"pred_fac_3", len(pred_fac_3), 10},
		{"dico1_lsf_3", len(dico1_lsf_3), 256 * 3},
		{"dico2_lsf_3", len(dico2_lsf_3), 512 * 3},
		{"dico3_lsf_3", len(dico3_lsf_3), 512 * 4},
		{"mr515_3_lsf", len(mr515_3_lsf), 128 * 4},
		{"mr795_1_lsf", len(mr795_1_lsf), 512 * 3},
		{"mean_lsf_5", len(mean_lsf_5), 10},
		{"dico1_lsf_5", len(dico1_lsf_5), 128 * 4},
		{"dico2_lsf_5", len(dico2_lsf_5), 256 * 4},
		{"dico3_lsf_5", len(dico3_lsf_5), 256 * 4},
		{"dico4_lsf_5", len(dico4_lsf_5), 256 * 4},
		{"dico5_lsf_5", len(dico5_lsf_5), 64 * 4},
		{"lspLsfTable", len(lspLsfTable), 65},
		{"lspLsfSlope", len(lspLsfSlope), 64},
		{"lsp_init_data", len(lsp_init_data), 10},
		{"grid", len(grid), 61},
		// Gain quantizer (gains_tbl.cpp / qua_gain_tbl.cpp)
		{"qua_gain_pitch", len(qua_gain_pitch), 16},
		{"qua_gain_code", len(qua_gain_code), 99},
		{"table_gain_highrates", len(table_gain_highrates), 128 * 4},
		{"table_gain_lowrates", len(table_gain_lowrates), 64 * 4},
		// Codebook / phase dispersion (c2_9pf_tab / gray_tbl / ph_disp_tab)
		{"startPos", len(startPos), 16},
		{"gray", len(gray), 8},
		{"dgray", len(dgray), 8},
		{"ph_imp_low_MR795", len(ph_imp_low_MR795), 40},
		{"ph_imp_mid_MR795", len(ph_imp_mid_MR795), 40},
		{"ph_imp_low", len(ph_imp_low), 40},
		{"ph_imp_mid", len(ph_imp_mid), 40},
		// LP analysis windows (window_tab.cpp)
		{"window_200_40", len(window_200_40), 240},
		{"window_160_80", len(window_160_80), 240},
		{"window_232_8", len(window_232_8), 240},
		// Math tables
		{"log2_tbl", len(log2_tbl), 33},
		{"pow2_tbl", len(pow2_tbl), 33},
		{"inv_sqrt_tbl", len(inv_sqrt_tbl), 49},
		{"sqrt_l_tbl", len(sqrt_l_tbl), 50},
		{"overflow_tbl", len(overflow_tbl), 32},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: len %d, want %d", c.name, c.got, c.want)
		}
	}
}

// TestTableAnchors spot-checks known boundary values to catch a misaligned
// extraction (e.g. an off-by-one row or a leaked cast token).
func TestTableAnchors(t *testing.T) {
	checks := []struct {
		name string
		got  int16
		want int16
	}{
		{"lspLsfTable[0]", lspLsfTable[0], 32767},
		{"lspLsfTable[64]", lspLsfTable[64], -32768}, // (Word16)0x8000
		{"grid[0]", grid[0], 32760},
		{"lsp_init_data[0]", lsp_init_data[0], 30000},
		{"lsp_init_data[5]", lsp_init_data[5], 0},
		{"gray[2]", gray[2], 3},
		{"dgray[4]", dgray[4], 5}, // inverse of gray (gray[5]=4 -> dgray[4]=5)
		{"qua_gain_pitch[0]", qua_gain_pitch[0], 0},
		{"qua_gain_pitch[3]", qua_gain_pitch[3], 8192},
		{"mean_lsf_3[0]", mean_lsf_3[0], 1546},
		{"log2_tbl[0]", log2_tbl[0], 0},
		{"pow2_tbl[0]", pow2_tbl[0], 16384},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.name, c.got, c.want)
		}
	}
}

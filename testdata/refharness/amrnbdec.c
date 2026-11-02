/*
 * Reference decoder harness for the differential tests: stock opencore-amrnb
 * (Apache-2.0) driven through its public Decoder_Interface_* API.
 *
 *   amrnbdec          stdin: frames as [uint16 LE length][storage frame:
 *                     ToC byte + speech or SID bytes], so mode and frame type
 *                     can change per frame.
 *   amrnbdec <mode>   stdin: packed speech bytes only, fixed mode 0..7.
 *
 * stdout: 160 int16 LE samples per frame.
 */
#include <stdio.h>
#include <stdlib.h>
#include <stdint.h>
#include <string.h>

/* opencore-amrnb/interf_dec.h, declared here so only the runtime library is needed. */
void *Decoder_Interface_init(void);
void Decoder_Interface_Decode(void *state, const unsigned char *in, short *out, int bfi);
void Decoder_Interface_exit(void *state);

static const int speech_bytes[8] = {12, 13, 15, 17, 19, 20, 26, 31};

int main(int argc, char **argv) {
    void *d = Decoder_Interface_init();
    uint8_t f[64];
    int16_t out[160];

    if (argc > 1) {
        int mode = atoi(argv[1]);
        if (mode < 0 || mode > 7) {
            fprintf(stderr, "mode must be 0..7\n");
            return 2;
        }
        int n = speech_bytes[mode];
        f[0] = (uint8_t)(mode << 3 | 0x04);
        while (fread(f + 1, 1, n, stdin) == (size_t)n) {
            Decoder_Interface_Decode(d, f, out, 0);
            fwrite(out, 2, 160, stdout);
        }
    } else {
        uint8_t len[2];
        while (fread(len, 1, 2, stdin) == 2) {
            int n = len[0] | len[1] << 8;
            if (n < 1 || n > (int)sizeof f || fread(f, 1, n, stdin) != (size_t)n) {
                fprintf(stderr, "bad frame length %d\n", n);
                return 1;
            }
            memset(out, 0, sizeof out);
            Decoder_Interface_Decode(d, f, out, 0);
            fwrite(out, 2, 160, stdout);
        }
    }
    Decoder_Interface_exit(d);
    return 0;
}

/*
 * Reference encoder harness for the differential tests: stock opencore-amrnb
 * (Apache-2.0) driven through its public Encoder_Interface_* API.
 *
 *   amrnbenc [-dtx] <mode>   stdin: 160 int16 LE samples per frame, fixed mode
 *                            0..7.
 *   amrnbenc [-dtx]          stdin: per frame, one mode byte (0..7) then 160
 *                            int16 LE samples, so the mode can change per frame.
 *
 * -dtx turns DTX on (opencore-amrnb sets it once, at init).
 *
 * stdout: storage frames (ToC byte + speech or SID bytes; ToC only for
 * NO_DATA).
 */
#include <stdio.h>
#include <stdlib.h>
#include <stdint.h>
#include <string.h>

/* opencore-amrnb/interf_enc.h, declared here so only the runtime library is needed. */
void *Encoder_Interface_init(int dtx);
int Encoder_Interface_Encode(void *state, int mode, const short *speech, unsigned char *out, int forceSpeech);
void Encoder_Interface_exit(void *state);

int main(int argc, char **argv) {
    int mode = -1, dtx = 0;
    for (int i = 1; i < argc; i++) {
        if (strcmp(argv[i], "-dtx") == 0) {
            dtx = 1;
            continue;
        }
        mode = atoi(argv[i]);
        if (mode < 0 || mode > 7) {
            fprintf(stderr, "mode must be 0..7\n");
            return 2;
        }
    }
    void *e = Encoder_Interface_init(dtx);
    int16_t in[160];
    uint8_t out[64];
    for (;;) {
        int m = mode;
        if (m < 0) {
            uint8_t b;
            if (fread(&b, 1, 1, stdin) != 1)
                break;
            if (b > 7) {
                fprintf(stderr, "bad mode byte %d\n", b);
                return 1;
            }
            m = b;
        }
        if (fread(in, 2, 160, stdin) != 160)
            break;
        int n = Encoder_Interface_Encode(e, m, in, out, 0);
        fwrite(out, 1, n, stdout);
    }
    Encoder_Interface_exit(e);
    return 0;
}

#include <stdint.h>
#include <string.h>

int64_t add(int64_t a, int64_t b) {
    /* Unsigned arithmetic defines wrapping; memcpy preserves the signed bits. */
    uint64_t result = (uint64_t)a + (uint64_t)b;
    volatile uint64_t x = result;
    for (int i = 0; i < 10000; i++) {
        x ^= x << 13;
        x ^= x >> 17;
        x ^= x << 5;
    }
    int64_t signed_result;
    memcpy(&signed_result, &result, sizeof(result));
    return signed_result;
}

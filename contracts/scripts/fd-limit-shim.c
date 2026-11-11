// Preloaded into the nearcore sandbox by sandbox-fd-shim.sh.
//
// nearcore sets its open-file limit (RLIMIT_NOFILE) to 65,535 at startup and
// exits if that fails. In containers whose hard limit is lower and cannot be
// raised (no CAP_SYS_RESOURCE), this library clamps the request to the
// current hard limit, which is plenty for a local test node. Other limits are
// passed through unchanged.
#define _GNU_SOURCE
#include <dlfcn.h>
#include <sys/resource.h>

static void clamp(int resource, unsigned long long *cur, unsigned long long *max) {
    struct rlimit hard;
    if (resource == RLIMIT_NOFILE && getrlimit(RLIMIT_NOFILE, &hard) == 0) {
        if (*max > hard.rlim_max) *max = hard.rlim_max;
        if (*cur > *max) *cur = *max;
    }
}

int setrlimit(__rlimit_resource_t resource, const struct rlimit *rlim) {
    static int (*real)(__rlimit_resource_t, const struct rlimit *);
    if (!real) real = dlsym(RTLD_NEXT, "setrlimit");
    unsigned long long cur = rlim->rlim_cur, max = rlim->rlim_max;
    clamp(resource, &cur, &max);
    struct rlimit want = {cur, max};
    return real(resource, &want);
}

int setrlimit64(__rlimit_resource_t resource, const struct rlimit64 *rlim) {
    static int (*real)(__rlimit_resource_t, const struct rlimit64 *);
    if (!real) real = dlsym(RTLD_NEXT, "setrlimit64");
    unsigned long long cur = rlim->rlim_cur, max = rlim->rlim_max;
    clamp(resource, &cur, &max);
    struct rlimit64 want = {cur, max};
    return real(resource, &want);
}

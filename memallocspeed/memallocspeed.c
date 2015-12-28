// for https://groups.google.com/forum/#!topic/golang-nuts/HZ_sBoLtnX4 
#include <stdio.h>
#include <sys/time.h>
#include <stdlib.h>
#include <string.h>

char zero[256*8];

double
now(void)
{
    struct timeval tv;
    gettimeofday(&tv, 0);
    return (double)tv.tv_sec+tv.tv_usec/1e6;
}

int 
main(void)
{
    char *p;
    long long size, i;
    double start, t;

    start = now();
    size = 1<<30;
    for(;;) {
        p = malloc(size);
        if (p==NULL) {
                printf("malloc failed, press <CTL+C> to exit.\n");
                getchar();
                return 1;
        }
        for(i=0; i<size; i+=sizeof zero){
            memmove(p+i, zero, sizeof zero);
        }
        t = now() - start;
        printf("%lld MB at %.2f = %.2f MB/s\n", size>>20, t, size/(double)(1<<20)/t);
        size += size/4;
    }

    return 0;
}

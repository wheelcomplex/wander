#define _GNU_SOURCE
#include <unistd.h>
#include <stdlib.h>
#include <stdio.h>
#include <strings.h>
#include <strings.h>
#include <stdint.h>

#define SIZE(array) (int)((sizeof array)/(sizeof *array))

int IntArraySize(const int *array) {
        return SIZE(array);
}

// array_create create array in length
uint32_t* array_create(uint32_t length){
	uint32_t i = 0;
	uint32_t* array;
	array = (uint32_t*)malloc( length*sizeof(uint32_t));
     if(!array){
        printf("C.array_create(%d): %p, failed\n", length, array);
        exit(1); 
    	}
	// initial
     bzero(array, length);
	printf("C.array_create: %d || %p || %x\n", array, array, array);
	for (i = 0; i < length; i ++) {
		printf("C.array_create(%d): %d -> %d\n", length, i, array[i]);
	}
	return array;
}

// array_show printf all array items
uint32_t array_show(uint32_t length, uint32_t* array){
	uint32_t i = 0;
	for (i = 0; i < length; i ++) {
		printf("C.array_show(%d): %d -> %d\n", length, i, array[i]);
	}
	return length;
}

// array_modify change all array items
uint32_t array_modify(uint32_t length, uint32_t* array, uint32_t val){
	uint32_t i = 0;
	for (i = 0; i < length; i ++) {
		printf("befor C.array_modify(%d): %d -> %d\n", length, i, array[i]);
	}
	for (i = 0; i < length; i ++) {
		array[i] = val;
		printf("after C.array_modify(%d): %d -> %d\n", length, i, array[i]);
	}
	return length;
}
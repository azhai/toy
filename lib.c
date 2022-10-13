
// Want to call a custom C function from kaleidoscope?
// Good news! Here's how:
// <detailed instructions>

#include <stdio.h>

double println(double x) {
	printf("result: %0.6f\n", x);
	fflush(stdout);
	return x;
}

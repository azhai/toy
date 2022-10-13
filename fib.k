
extern println(x)

# Declared Variables
def binary:1(x,y) y
def fibi(x)
  var a = 0, b = 1, c in
  (for i = 2, i < x in
     c = a + b :
     a = b :
     b = c ) :
  b;

def main()
  println(fibi(20))
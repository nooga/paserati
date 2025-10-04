var caught = false;
var isTypeError = false;
try {
  true();
} catch (e) {
  caught = true;
  isTypeError = (e instanceof TypeError);
  print("Caught exception:", e);
  print("Name:", e.name);
  print("Message:", e.message);
  print("instanceof TypeError:", isTypeError);
}

if (!caught) {
  print("ERROR: No exception was caught");
}
if (!isTypeError) {
  print("ERROR: Exception is not a TypeError");
}

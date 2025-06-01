// expect: 0
const obj = {
  val: 10,
  countdown() {
    if (this.val > 0) {
      this.val = this.val - 1;
      this.countdown();
    }
    return this.val;
  },
};

obj.countdown();

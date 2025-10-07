// expect: pass
var C = class {
  static;
  public = 1;
  private = 2;
  protected = 3;
  readonly = 4;
}

var c = new C();
(c.static === undefined && c.public === 1 && c.private === 2 &&
 c.protected === 3 && c.readonly === 4) ? "pass" : "fail";

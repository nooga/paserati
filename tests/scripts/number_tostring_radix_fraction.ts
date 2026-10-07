// expect: 0.1|ff.c|4fzzzx|-0.01|3.4|3.53i5ab8p5f|0.0001100110011001100110011001100110011001100110011001101
[(0.5).toString(2), (255.75).toString(16), (0.123456789).toString(36).substring(2, 8), (-0.25).toString(2), (3.5).toString(8), Math.PI.toString(36), (0.1).toString(2)].join("|");

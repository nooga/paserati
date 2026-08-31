// expect: true
// no-typecheck

// The TDZ fix for #143's class pre-registration also closed a pre-existing gap
// it sits on top of: SPILLED let/const bindings had no TDZ check at all.
//
// Both `symbolRef.IsSpilled` identifier branches in compiler.go emitted a bare
// OpLoadSpill with no OpCheckUninitialized, unlike the two register-based
// paths right next to them. OpLoadSpill has no VM-side TDZ guard either (only
// OpLoadFree does), so reading a spilled let/const before its declaration
// quietly produced the raw TDZ sentinel instead of throwing ReferenceError.
// The identical code in a function small enough to keep everything in
// registers threw correctly - it only broke once register pressure forced the
// binding into a spill slot, which is why no existing test caught it.
//
// Hence the 200 dummy locals below: they are not incidental, they are what
// pushes `late` into a spill slot. Measured threshold on this VM - at 150
// locals the pre-fix build still returns true (registers, working check), at
// 200 it returns false (spilled, no check). Do not "tidy" the count down.
//
// Verified to discriminate: on a88125c2 (pre-#143) this returns false; with
// either half of the compiler.go TDZ fix reverted it returns false.

function spilledLetTDZ(): boolean {

    let v0 = 0;
    let v1 = 1;
    let v2 = 2;
    let v3 = 3;
    let v4 = 4;
    let v5 = 5;
    let v6 = 6;
    let v7 = 7;
    let v8 = 8;
    let v9 = 9;
    let v10 = 10;
    let v11 = 11;
    let v12 = 12;
    let v13 = 13;
    let v14 = 14;
    let v15 = 15;
    let v16 = 16;
    let v17 = 17;
    let v18 = 18;
    let v19 = 19;
    let v20 = 20;
    let v21 = 21;
    let v22 = 22;
    let v23 = 23;
    let v24 = 24;
    let v25 = 25;
    let v26 = 26;
    let v27 = 27;
    let v28 = 28;
    let v29 = 29;
    let v30 = 30;
    let v31 = 31;
    let v32 = 32;
    let v33 = 33;
    let v34 = 34;
    let v35 = 35;
    let v36 = 36;
    let v37 = 37;
    let v38 = 38;
    let v39 = 39;
    let v40 = 40;
    let v41 = 41;
    let v42 = 42;
    let v43 = 43;
    let v44 = 44;
    let v45 = 45;
    let v46 = 46;
    let v47 = 47;
    let v48 = 48;
    let v49 = 49;
    let v50 = 50;
    let v51 = 51;
    let v52 = 52;
    let v53 = 53;
    let v54 = 54;
    let v55 = 55;
    let v56 = 56;
    let v57 = 57;
    let v58 = 58;
    let v59 = 59;
    let v60 = 60;
    let v61 = 61;
    let v62 = 62;
    let v63 = 63;
    let v64 = 64;
    let v65 = 65;
    let v66 = 66;
    let v67 = 67;
    let v68 = 68;
    let v69 = 69;
    let v70 = 70;
    let v71 = 71;
    let v72 = 72;
    let v73 = 73;
    let v74 = 74;
    let v75 = 75;
    let v76 = 76;
    let v77 = 77;
    let v78 = 78;
    let v79 = 79;
    let v80 = 80;
    let v81 = 81;
    let v82 = 82;
    let v83 = 83;
    let v84 = 84;
    let v85 = 85;
    let v86 = 86;
    let v87 = 87;
    let v88 = 88;
    let v89 = 89;
    let v90 = 90;
    let v91 = 91;
    let v92 = 92;
    let v93 = 93;
    let v94 = 94;
    let v95 = 95;
    let v96 = 96;
    let v97 = 97;
    let v98 = 98;
    let v99 = 99;
    let v100 = 100;
    let v101 = 101;
    let v102 = 102;
    let v103 = 103;
    let v104 = 104;
    let v105 = 105;
    let v106 = 106;
    let v107 = 107;
    let v108 = 108;
    let v109 = 109;
    let v110 = 110;
    let v111 = 111;
    let v112 = 112;
    let v113 = 113;
    let v114 = 114;
    let v115 = 115;
    let v116 = 116;
    let v117 = 117;
    let v118 = 118;
    let v119 = 119;
    let v120 = 120;
    let v121 = 121;
    let v122 = 122;
    let v123 = 123;
    let v124 = 124;
    let v125 = 125;
    let v126 = 126;
    let v127 = 127;
    let v128 = 128;
    let v129 = 129;
    let v130 = 130;
    let v131 = 131;
    let v132 = 132;
    let v133 = 133;
    let v134 = 134;
    let v135 = 135;
    let v136 = 136;
    let v137 = 137;
    let v138 = 138;
    let v139 = 139;
    let v140 = 140;
    let v141 = 141;
    let v142 = 142;
    let v143 = 143;
    let v144 = 144;
    let v145 = 145;
    let v146 = 146;
    let v147 = 147;
    let v148 = 148;
    let v149 = 149;
    let v150 = 150;
    let v151 = 151;
    let v152 = 152;
    let v153 = 153;
    let v154 = 154;
    let v155 = 155;
    let v156 = 156;
    let v157 = 157;
    let v158 = 158;
    let v159 = 159;
    let v160 = 160;
    let v161 = 161;
    let v162 = 162;
    let v163 = 163;
    let v164 = 164;
    let v165 = 165;
    let v166 = 166;
    let v167 = 167;
    let v168 = 168;
    let v169 = 169;
    let v170 = 170;
    let v171 = 171;
    let v172 = 172;
    let v173 = 173;
    let v174 = 174;
    let v175 = 175;
    let v176 = 176;
    let v177 = 177;
    let v178 = 178;
    let v179 = 179;
    let v180 = 180;
    let v181 = 181;
    let v182 = 182;
    let v183 = 183;
    let v184 = 184;
    let v185 = 185;
    let v186 = 186;
    let v187 = 187;
    let v188 = 188;
    let v189 = 189;
    let v190 = 190;
    let v191 = 191;
    let v192 = 192;
    let v193 = 193;
    let v194 = 194;
    let v195 = 195;
    let v196 = 196;
    let v197 = 197;
    let v198 = 198;
    let v199 = 199;

    // Read `late` before its declaration. Must throw ReferenceError.
    let caught = false;
    try {
        const t = late;
    } catch (e) {
        caught = e instanceof ReferenceError;
    }
    let late = 1;

    // Keep every dummy local live so they actually occupy registers.
    const sum = v0 + v1 + v2 + v3 + v4 + v5 + v6 + v7 + v8 + v9 + v10 + v11 + v12 + v13 + v14 + v15 + v16 + v17 + v18 + v19 + v20 + v21 + v22 + v23 + v24 + v25 + v26 + v27 + v28 + v29 + v30 + v31 + v32 + v33 + v34 + v35 + v36 + v37 + v38 + v39 + v40 + v41 + v42 + v43 + v44 + v45 + v46 + v47 + v48 + v49 + v50 + v51 + v52 + v53 + v54 + v55 + v56 + v57 + v58 + v59 + v60 + v61 + v62 + v63 + v64 + v65 + v66 + v67 + v68 + v69 + v70 + v71 + v72 + v73 + v74 + v75 + v76 + v77 + v78 + v79 + v80 + v81 + v82 + v83 + v84 + v85 + v86 + v87 + v88 + v89 + v90 + v91 + v92 + v93 + v94 + v95 + v96 + v97 + v98 + v99 + v100 + v101 + v102 + v103 + v104 + v105 + v106 + v107 + v108 + v109 + v110 + v111 + v112 + v113 + v114 + v115 + v116 + v117 + v118 + v119 + v120 + v121 + v122 + v123 + v124 + v125 + v126 + v127 + v128 + v129 + v130 + v131 + v132 + v133 + v134 + v135 + v136 + v137 + v138 + v139 + v140 + v141 + v142 + v143 + v144 + v145 + v146 + v147 + v148 + v149 + v150 + v151 + v152 + v153 + v154 + v155 + v156 + v157 + v158 + v159 + v160 + v161 + v162 + v163 + v164 + v165 + v166 + v167 + v168 + v169 + v170 + v171 + v172 + v173 + v174 + v175 + v176 + v177 + v178 + v179 + v180 + v181 + v182 + v183 + v184 + v185 + v186 + v187 + v188 + v189 + v190 + v191 + v192 + v193 + v194 + v195 + v196 + v197 + v198 + v199;

    // ...and the binding must work normally once initialized.
    return caught && sum === 19900 && late === 1;
}

spilledLetTDZ();

export interface WeeklyLanguageData {
  week: string;
  french: number;
  german: number;
  japanese: number;
}

export interface WeeklyData {
  week: string;
  value: number;
}

export interface AcceptanceData {
  translator: string;
  accepted: number;
  edited: number;
  rejected: number;
  color: string;
}

export interface HeatmapCell {
  week: number;
  day: number;
  count: number;
}

export const translationProgress: WeeklyLanguageData[] = [
  { week: "W1", french: 5, german: 3, japanese: 1 },
  { week: "W2", french: 12, german: 8, japanese: 4 },
  { week: "W3", french: 22, german: 16, japanese: 9 },
  { week: "W4", french: 31, german: 24, japanese: 14 },
  { week: "W5", french: 40, german: 31, japanese: 20 },
  { week: "W6", french: 48, german: 38, japanese: 26 },
  { week: "W7", french: 55, german: 44, japanese: 31 },
  { week: "W8", french: 63, german: 51, japanese: 36 },
  { week: "W9", french: 70, german: 57, japanese: 40 },
  { week: "W10", french: 76, german: 62, japanese: 44 },
  { week: "W11", french: 81, german: 68, japanese: 48 },
  { week: "W12", french: 79, german: 67, japanese: 48 },
];

export const tmGrowth: WeeklyData[] = [
  { week: "W1", value: 120 },
  { week: "W2", value: 385 },
  { week: "W3", value: 720 },
  { week: "W4", value: 1150 },
  { week: "W5", value: 1680 },
  { week: "W6", value: 2340 },
  { week: "W7", value: 3010 },
  { week: "W8", value: 3750 },
  { week: "W9", value: 4480 },
  { week: "W10", value: 5210 },
  { week: "W11", value: 5980 },
  { week: "W12", value: 7257 },
];

export const qualityScores: WeeklyLanguageData[] = [
  { week: "W1", french: 82, german: 78, japanese: 71 },
  { week: "W2", french: 84, german: 80, japanese: 73 },
  { week: "W3", french: 87, german: 83, japanese: 76 },
  { week: "W4", french: 89, german: 85, japanese: 78 },
  { week: "W5", french: 91, german: 87, japanese: 80 },
  { week: "W6", french: 92, german: 88, japanese: 82 },
  { week: "W7", french: 93, german: 90, japanese: 84 },
  { week: "W8", french: 94, german: 91, japanese: 86 },
  { week: "W9", french: 95, german: 92, japanese: 88 },
  { week: "W10", french: 96, german: 93, japanese: 90 },
  { week: "W11", french: 97, german: 94, japanese: 91 },
  { week: "W12", french: 98, german: 95, japanese: 93 },
];

export const costEfficiency: WeeklyData[] = [
  { week: "W1", value: 0.12 },
  { week: "W2", value: 0.11 },
  { week: "W3", value: 0.098 },
  { week: "W4", value: 0.088 },
  { week: "W5", value: 0.079 },
  { week: "W6", value: 0.071 },
  { week: "W7", value: 0.064 },
  { week: "W8", value: 0.058 },
  { week: "W9", value: 0.053 },
  { week: "W10", value: 0.048 },
  { week: "W11", value: 0.044 },
  { week: "W12", value: 0.041 },
];

export const aiAcceptanceRates: AcceptanceData[] = [
  { translator: "Jean-Pierre", accepted: 60, edited: 28, rejected: 12, color: "#3b82f6" },
  { translator: "Katrin", accepted: 40, edited: 35, rejected: 25, color: "#f43f5e" },
  { translator: "Yuki", accepted: 30, edited: 42, rejected: 28, color: "#8b5cf6" },
];

export const activityHeatmap: HeatmapCell[] = (() => {
  const cells: HeatmapCell[] = [];
  const patterns = [
    [3, 5, 4, 6, 5, 2, 0],
    [4, 6, 5, 7, 6, 3, 1],
    [5, 7, 6, 8, 7, 4, 1],
    [6, 8, 7, 9, 8, 4, 2],
    [7, 9, 8, 10, 9, 5, 2],
    [8, 10, 9, 12, 10, 6, 3],
    [9, 11, 10, 13, 11, 6, 3],
    [10, 12, 11, 14, 12, 7, 4],
    [11, 14, 12, 15, 13, 8, 4],
    [12, 15, 13, 16, 14, 8, 5],
    [13, 16, 14, 18, 15, 9, 5],
    [14, 17, 15, 19, 16, 10, 6],
  ];
  for (let w = 0; w < 12; w++) {
    for (let d = 0; d < 7; d++) {
      cells.push({ week: w, day: d, count: patterns[w][d] });
    }
  }
  return cells;
})();

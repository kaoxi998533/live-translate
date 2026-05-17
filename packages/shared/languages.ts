export type TranslationLanguage = {
  code: string;
  label: string;
  englishName: string;
};

export const sourceLanguages: TranslationLanguage[] = [
  { code: "auto", label: "自动识别", englishName: "the detected source language" },
  { code: "en", label: "英语", englishName: "English" },
  { code: "zh", label: "中文", englishName: "Mandarin Chinese" },
  { code: "ja", label: "日语", englishName: "Japanese" },
  { code: "ko", label: "韩语", englishName: "Korean" },
  { code: "es", label: "西班牙语", englishName: "Spanish" },
  { code: "fr", label: "法语", englishName: "French" },
  { code: "de", label: "德语", englishName: "German" },
  { code: "it", label: "意大利语", englishName: "Italian" },
  { code: "pt", label: "葡萄牙语", englishName: "Portuguese" },
  { code: "ru", label: "俄语", englishName: "Russian" },
  { code: "ar", label: "阿拉伯语", englishName: "Arabic" },
  { code: "hi", label: "印地语", englishName: "Hindi" },
  { code: "id", label: "印尼语", englishName: "Indonesian" },
  { code: "th", label: "泰语", englishName: "Thai" },
  { code: "vi", label: "越南语", englishName: "Vietnamese" }
];

export const targetLanguages: TranslationLanguage[] = sourceLanguages.filter(
  (language) => language.code !== "auto"
);

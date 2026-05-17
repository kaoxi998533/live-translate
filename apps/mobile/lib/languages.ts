export type TranslationLanguage = {
  code: string;
  label: string;
  englishName: string;
};

export const sourceLanguages: TranslationLanguage[] = [
  { code: "auto", label: "Auto detect", englishName: "the detected source language" },
  { code: "en", label: "English", englishName: "English" },
  { code: "zh", label: "中文", englishName: "Mandarin Chinese" },
  { code: "ja", label: "日本語", englishName: "Japanese" },
  { code: "ko", label: "한국어", englishName: "Korean" },
  { code: "es", label: "Español", englishName: "Spanish" },
  { code: "fr", label: "Français", englishName: "French" },
  { code: "de", label: "Deutsch", englishName: "German" },
  { code: "it", label: "Italiano", englishName: "Italian" },
  { code: "pt", label: "Português", englishName: "Portuguese" },
  { code: "ru", label: "Русский", englishName: "Russian" },
  { code: "ar", label: "العربية", englishName: "Arabic" },
  { code: "hi", label: "हिन्दी", englishName: "Hindi" },
  { code: "id", label: "Indonesia", englishName: "Indonesian" },
  { code: "th", label: "ไทย", englishName: "Thai" },
  { code: "vi", label: "Tiếng Việt", englishName: "Vietnamese" }
];

export const targetLanguages: TranslationLanguage[] = sourceLanguages.filter(
  (language) => language.code !== "auto"
);

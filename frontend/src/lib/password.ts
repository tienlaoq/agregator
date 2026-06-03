// Единая политика паролей для клиента. Совпадает с серверной проверкой в
// auth-service (usecase: минимум 8 символов + буква + цифра). Клиент даёт
// мгновенную обратную связь, сервер — финальный барьер (нельзя обойти через API).

export const PASSWORD_MIN_LENGTH = 8;

export interface PasswordRule {
  /** Стабильный ключ требования. */
  key: "length" | "letter" | "digit";
  /** Человекочитаемая подпись для чек-листа. */
  label: string;
  /** Выполнено ли требование для данного пароля. */
  test: (pw: string) => boolean;
}

/** Обязательные требования — пока не выполнены все, регистрация блокируется. */
export const PASSWORD_RULES: PasswordRule[] = [
  {
    key: "length",
    label: `Не менее ${PASSWORD_MIN_LENGTH} символов`,
    test: (pw) => pw.length >= PASSWORD_MIN_LENGTH,
  },
  {
    key: "letter",
    label: "Хотя бы одна буква",
    test: (pw) => /\p{L}/u.test(pw),
  },
  {
    key: "digit",
    label: "Хотя бы одна цифра",
    test: (pw) => /\d/.test(pw),
  },
];

/** Пароль проходит обязательную политику (можно отправлять на сервер). */
export function isPasswordValid(pw: string): boolean {
  return PASSWORD_RULES.every((r) => r.test(pw));
}

export type PasswordStrength = "weak" | "medium" | "strong";

export interface PasswordStrengthResult {
  /** Нормированный уровень для индикатора. */
  level: PasswordStrength;
  /** 0..4 — для отрисовки шкалы. */
  score: 0 | 1 | 2 | 3 | 4;
  label: string;
}

/**
 * Оценка «сложности» для индикатора. Это подсказка, а не блокирующее правило:
 * блокирует только isPasswordValid(). Учитываем длину и разнообразие символов.
 */
export function evaluatePasswordStrength(pw: string): PasswordStrengthResult {
  if (!pw) return { level: "weak", score: 0, label: "" };

  let points = 0;
  if (pw.length >= PASSWORD_MIN_LENGTH) points += 1;
  if (pw.length >= 12) points += 1;
  // Разнообразие классов символов.
  const classes = [
    /[a-zа-яё]/u.test(pw), // строчные
    /[A-ZА-ЯЁ]/u.test(pw), // заглавные
    /\d/.test(pw), // цифры
    /[^\p{L}\d]/u.test(pw), // спецсимволы
  ].filter(Boolean).length;
  if (classes >= 2) points += 1;
  if (classes >= 3) points += 1;

  // Слишком короткий пароль не может быть «сильным», даже с разными символами.
  if (pw.length < PASSWORD_MIN_LENGTH) {
    return { level: "weak", score: 1, label: "Слабый" };
  }

  const score = Math.min(4, points) as 0 | 1 | 2 | 3 | 4;
  if (score <= 2) return { level: "weak", score, label: "Слабый" };
  if (score === 3) return { level: "medium", score, label: "Средний" };
  return { level: "strong", score, label: "Надёжный" };
}

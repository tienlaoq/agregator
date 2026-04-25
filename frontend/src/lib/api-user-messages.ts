/**
 * User-facing Russian copy for api-gateway machine codes.
 * Never surface JSON `error` (may contain gRPC / English internals).
 * Keep in sync with services/api-gateway/internal/apicatalog/errors.yaml.
 */
export const USER_FACING_MESSAGE_BY_GATEWAY_CODE: Record<string, string> = {
  "GATEWAY.AUTH.UNAUTHORIZED": "Сначала войдите в аккаунт.",
  "GATEWAY.REQUEST.INVALID_BODY": "Запрос заполнен неверно. Обновите страницу и попробуйте снова.",
  "GATEWAY.REQUEST.INVALID_JSON": "Данные в неверном формате. Обновите страницу и попробуйте снова.",
  "GATEWAY.REQUEST.INVALID_QUERY": "Неверные параметры в адресе или фильтрах.",
  "GATEWAY.REQUEST.MISSING_DATE": "Укажите дату.",
  "GATEWAY.REQUEST.MISSING_DATE_RANGE": "Укажите период: дату начала и дату окончания.",
  "GATEWAY.REQUEST.EMAIL_REQUIRED": "Укажите email.",
  "GATEWAY.VENUE.NOT_FOUND": "Такого заведения нет или оно скрыто.",
  "GATEWAY.DEPENDENCY.USER_SERVICE_UNAVAILABLE": "Сервис сейчас недоступен. Попробуйте позже.",
  "GATEWAY.REQUEST.INVALID_VENUE_ID": "Неверная ссылка на заведение.",
  "GATEWAY.REQUEST.INVALID_HALL_ID": "Неверная ссылка на зал.",
  "GATEWAY.REQUEST.INVALID_PHOTO_ID": "Фото не найдено или ссылка устарела.",
  "GATEWAY.REQUEST.INVALID_MULTIPART": "Файл не подошёл: проверьте размер и форму отправки.",
  "GATEWAY.REQUEST.PHOTO_FIELD_REQUIRED": "Выберите файл для загрузки.",
  "GATEWAY.REQUEST.INVALID_FILE_READ": "Не удалось прочитать файл. Попробуйте другой файл.",
  "GATEWAY.REQUEST.EMPTY_FILE": "Файл пустой. Выберите другой файл.",
  "GATEWAY.REQUEST.INVALID_IMAGE_TYPE": "Подходят только изображения JPEG, PNG или WebP.",
  "GATEWAY.MASTER.NOT_CREATED": "Сначала создайте профиль мастера.",
  "GATEWAY.INTERNAL.INVALID_MASTER_ID": "Что-то пошло не так. Обновите страницу или обратитесь в поддержку.",
  "GATEWAY.STORAGE.FAILED": "Не удалось сохранить файл. Попробуйте позже.",
  "GATEWAY.BOOKING.TIME_FROM_REQUIRED": "Укажите время начала бронирования.",
  "GATEWAY.REVIEW.VENUE_ID_REQUIRED": "Не удалось определить заведение для отзыва.",
  "GATEWAY.REQUEST.BODY_READ_FAILED": "Не удалось обработать запрос. Обновите страницу и попробуйте снова.",
  "GATEWAY.REQUEST.METHOD_NOT_ALLOWED": "Это действие сейчас недоступно.",
  "GATEWAY.ANALYTICS.INVALID_EVENT_NAME": "Не удалось записать действие. Обновите страницу.",
  "GATEWAY.ANALYTICS.PROPS_INVALID_OR_TOO_LARGE": "Не удалось записать действие. Попробуйте позже.",
  "GATEWAY.OAUTH.GOOGLE_NOT_CONFIGURED": "Вход через Google сейчас недоступен.",
  "GATEWAY.OAUTH.VK_NOT_CONFIGURED": "Вход через ВКонтакте сейчас недоступен.",
  "GATEWAY.CRM.STAFF_EMAIL_NOT_REGISTERED":
    "На этот email ещё нет аккаунта — человек должен сначала зарегистрироваться.",
  "GATEWAY.MASTER.INVALID_SERVICES": "Проверьте список услуг и попробуйте снова.",
  "GATEWAY.UPSTREAM.INVALID_ARGUMENT": "Проверьте введённые данные и попробуйте снова.",
  "GATEWAY.UPSTREAM.NOT_FOUND": "Ничего не найдено по вашему запросу.",
  "GATEWAY.UPSTREAM.ALREADY_EXISTS": "Такая запись уже есть. Обновите страницу.",
  "GATEWAY.UPSTREAM.UNAUTHENTICATED": "Неверный email или пароль, либо сессия истекла. Войдите снова.",
  "GATEWAY.UPSTREAM.PERMISSION_DENIED": "У вас нет прав на это действие.",
  "GATEWAY.UPSTREAM.UNAVAILABLE": "Сервис временно недоступен. Попробуйте через несколько минут.",
  "GATEWAY.UPSTREAM.FAILED_PRECONDITION": "Сейчас это сделать нельзя. Обновите страницу или попробуйте позже.",
  "GATEWAY.UPSTREAM.INTERNAL": "Что-то пошло не так на стороне сервиса. Попробуйте позже.",
  "GATEWAY.UPSTREAM.UNKNOWN": "Что-то пошло не так. Попробуйте позже.",
};

const STATUS_FALLBACK: Partial<Record<number, string>> = {
  400: "Проверьте данные и попробуйте снова.",
  401: "Войдите в аккаунт или проверьте логин и пароль.",
  403: "У вас нет доступа к этому действию.",
  404: "Ничего не найдено по вашему запросу.",
  405: "Это действие сейчас недоступно.",
  409: "Не удалось выполнить: данные конфликтуют или условия не соблюдены.",
  422: "Проверьте данные и попробуйте снова.",
  500: "Что-то пошло не так. Попробуйте позже.",
  503: "Сервис временно недоступен. Попробуйте через несколько минут.",
};

export function userMessageForGatewayError(
  httpStatus: number,
  code: string | undefined,
  fallback: string,
): string {
  if (code && USER_FACING_MESSAGE_BY_GATEWAY_CODE[code]) {
    return USER_FACING_MESSAGE_BY_GATEWAY_CODE[code];
  }
  const byStatus = STATUS_FALLBACK[httpStatus];
  if (byStatus) return byStatus;
  return fallback;
}

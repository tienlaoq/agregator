"use client"

import { useState, useRef, useMemo } from "react"
import { usePathname, useRouter } from "next/navigation"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Badge } from "@/components/ui/badge"
import {
  register,
  createVenue,
  userFromRegisterResponse,
  ApiError,
  formatApiErrorMessage,
  isEmailNotVerifiedError,
} from "@/lib/api"
import { EmailVerificationNotice } from "@/components/banya/email-verification-notice"
import { useAuthStore } from "@/store/auth"
import { PhoneInput, getRawPhone } from "@/components/banya/phone-input"
import { PasswordStrength } from "@/components/banya/password-strength"
import { ConsentCheckbox } from "@/components/banya/consent-checkbox"
import { isPasswordValid, PASSWORD_MIN_LENGTH } from "@/lib/password"
import { PartnerVenueRegistrationCard } from "@/components/banya/partner-venue-registration-card"
import {
  buildPartnerStyleCreateVenueRequest,
  emptyPartnerVenueForm,
  partnerVenueStepCanProceed,
  venueTypeBadgeLabel,
  type PartnerVenueFormValues,
} from "@/lib/partner-venue"
import type { PartnerCabinetRole } from "@/lib/partner-roles"
import Link from "next/link"
import {
  Flame,
  ArrowRight,
  ArrowLeft,
  Users,
  TrendingUp,
  CalendarCheck,
  ShieldCheck,
  Building2,
  CheckCircle2,
  PartyPopper,
  Settings,
} from "lucide-react"

// Step 5 is the email-verification hold for the venue track: the account is
// already created and logged in, but createVenue was rejected by the email gate
// (EMAIL_NOT_VERIFIED). The draft is held in venueFields until the user clicks
// the link in the verification email and retries.
type Step = 1 | 2 | 3 | 4 | 5

export default function PartnerPage() {
  const router = useRouter()
  const pathname = usePathname()
  const partnerRole = useMemo<PartnerCabinetRole>(
    () => (pathname?.startsWith("/partner/master") ? "master" : "venue_owner"),
    [pathname],
  )
  const isMasterTrack = partnerRole === "master"

  const authLogin = useAuthStore((s) => s.login)
  const [step, setStep] = useState<Step>(1)
  const [error, setError] = useState("")
  const [loading, setLoading] = useState(false)

  const [venueFields, setVenueFields] = useState<PartnerVenueFormValues>(() => emptyPartnerVenueForm())
  const patchVenue = (p: Partial<PartnerVenueFormValues>) =>
    setVenueFields((prev) => ({ ...prev, ...p }))

  // Step 3: Contact data
  const [contactName, setContactName] = useState("")
  const [contactEmail, setContactEmail] = useState("")
  const [contactPhone, setContactPhone] = useState("")
  const [contactPassword, setContactPassword] = useState("")
  const [consent, setConsent] = useState(false)
  const passwordValid = isPasswordValid(contactPassword)

  const [createdVenueId, setCreatedVenueId] = useState("")
  const submittingRef = useRef(false)

  const handleSubmit = async () => {
    if (submittingRef.current) return
    submittingRef.current = true
    setError("")
    setLoading(true)
    try {
      const rawPhone = getRawPhone(contactPhone)
      const res = await register({
        name: contactName,
        email: contactEmail,
        phone: rawPhone,
        password: contactPassword,
        role: partnerRole,
      })
      localStorage.setItem("token", res.access_token)
      localStorage.setItem("refresh_token", res.refresh_token)
      const user = userFromRegisterResponse(res, {
        name: contactName,
        email: contactEmail,
        phone: rawPhone,
        role: partnerRole,
      })
      // ВАЖНО: дожидаемся authLogin до createVenue. login() кладёт токен в стор
      // только после await fetch(/api/auth/set-refresh); без await createVenue
      // ушёл бы со СТАРЫМ токеном из памяти (предыдущая сессия), и заведение
      // создавалось бы на прошлый аккаунт — а нового владельца перебрасывало на
      // «Заведение не найдено». См. гонку identity при повторных регистрациях.
      await authLogin(res.access_token, res.refresh_token, user)

      if (partnerRole === "master") {
        router.push("/owner/master/profile")
        return
      }

      const venue = await createVenue(buildPartnerStyleCreateVenueRequest(venueFields, rawPhone))
      setCreatedVenueId(venue.id)
      setStep(4)
    } catch (err) {
      // Account was created and we are logged in, but the email gate rejected
      // venue creation. Hold the draft and move to the verify-email step rather
      // than surfacing a scary error.
      if (isEmailNotVerifiedError(err)) {
        setError("")
        setStep(5)
      } else if (err instanceof ApiError) {
        setError(formatApiErrorMessage(err, "Не удалось создать аккаунт"))
      } else {
        setError(formatApiErrorMessage(err, "Не удалось подключиться к серверу"))
      }
    } finally {
      setLoading(false)
      submittingRef.current = false
    }
  }

  // Retry venue creation after the user confirms they clicked the verification
  // link. The account/session already exist, so we only re-run createVenue with
  // the held draft. Still gated → stay on step 5 with a hint.
  const handleVenueRetryAfterVerify = async () => {
    if (submittingRef.current) return
    submittingRef.current = true
    setError("")
    setLoading(true)
    try {
      const rawPhone = getRawPhone(contactPhone)
      const venue = await createVenue(buildPartnerStyleCreateVenueRequest(venueFields, rawPhone))
      setCreatedVenueId(venue.id)
      setStep(4)
    } catch (err) {
      if (isEmailNotVerifiedError(err)) {
        setError("Email пока не подтверждён. Откройте ссылку из письма и попробуйте снова.")
      } else if (err instanceof ApiError) {
        setError(formatApiErrorMessage(err, "Не удалось создать заведение"))
      } else {
        setError(formatApiErrorMessage(err, "Не удалось подключиться к серверу"))
      }
    } finally {
      setLoading(false)
      submittingRef.current = false
    }
  }

  return (
    <div className="min-h-[80vh] py-12">
      {/* Progress indicator */}
      {step !== 4 && step !== 5 && (
        <div className="container mx-auto mb-8 px-4">
          <div className="mx-auto flex max-w-2xl items-center justify-center gap-2">
            {(isMasterTrack ? [1, 3] : [1, 2, 3]).map((s, idx, arr) => (
              <div key={s} className="flex items-center gap-2">
                <div
                  className={`flex h-8 w-8 items-center justify-center rounded-full text-sm font-medium transition-colors ${
                    s === step
                      ? "bg-primary text-primary-foreground"
                      : s < step
                        ? "bg-primary/20 text-primary"
                        : "bg-muted text-muted-foreground"
                  }`}
                >
                  {s < step ? <CheckCircle2 className="h-5 w-5" /> : isMasterTrack ? idx + 1 : s}
                </div>
                <span className={`hidden text-sm sm:inline ${s === step ? "font-medium text-foreground" : "text-muted-foreground"}`}>
                  {s === 1
                    ? "Выгоды"
                    : isMasterTrack
                      ? "Контакты"
                      : s === 2
                        ? "Контакты"
                        : "О заведении"}
                </span>
                {idx < arr.length - 1 && (
                  <div
                    className={`h-px w-8 sm:w-16 ${arr[idx] < step ? "bg-primary" : "bg-border"}`}
                  />
                )}
              </div>
            ))}
          </div>
        </div>
      )}

      <div className="container mx-auto px-4">
        {/* Step 1: Benefits */}
        {step === 1 && (
          <div className="mx-auto max-w-3xl">
            <div className="mb-10 text-center">
              <div className="mx-auto mb-6 flex h-16 w-16 items-center justify-center rounded-full bg-primary">
                <Flame className="h-8 w-8 text-primary-foreground" />
              </div>
              <h1 className="mb-3 text-3xl font-bold text-foreground md:text-4xl">
                {isMasterTrack
                  ? "Профиль пар-мастера на БаняГид"
                  : "Развивайте бизнес с БаняГид"}
              </h1>
              <p className="mx-auto max-w-xl text-lg text-muted-foreground">
                {isMasterTrack
                  ? "Заполните профиль, пройдите модерацию и получайте заявки от клиентов. Пока профиль не одобрен, он не отображается в каталоге."
                  : "Присоединяйтесь к платформе, которой доверяют сотни заведений по всей России"}
              </p>
            </div>

            {/* Benefits grid */}
            <div className="mb-10 grid gap-6 sm:grid-cols-2">
              <Card className="border-border">
                <CardContent className="flex gap-4 p-6">
                  <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-lg bg-primary/10">
                    <Users className="h-6 w-6 text-primary" />
                  </div>
                  <div>
                    <h3 className="mb-1 font-semibold text-card-foreground">Новые клиенты</h3>
                    <p className="text-sm text-muted-foreground">
                      Тысячи людей ищут бани каждый день. Ваше заведение увидят все.
                    </p>
                  </div>
                </CardContent>
              </Card>

              <Card className="border-border">
                <CardContent className="flex gap-4 p-6">
                  <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-lg bg-primary/10">
                    <CalendarCheck className="h-6 w-6 text-primary" />
                  </div>
                  <div>
                    <h3 className="mb-1 font-semibold text-card-foreground">Онлайн-бронирование</h3>
                    <p className="text-sm text-muted-foreground">
                      Клиенты бронируют 24/7. Меньше звонков — больше посещений.
                    </p>
                  </div>
                </CardContent>
              </Card>

              <Card className="border-border">
                <CardContent className="flex gap-4 p-6">
                  <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-lg bg-primary/10">
                    <TrendingUp className="h-6 w-6 text-primary" />
                  </div>
                  <div>
                    <h3 className="mb-1 font-semibold text-card-foreground">Аналитика</h3>
                    <p className="text-sm text-muted-foreground">
                      Статистика бронирований, отзывов и выручки в личном кабинете.
                    </p>
                  </div>
                </CardContent>
              </Card>

              <Card className="border-border">
                <CardContent className="flex gap-4 p-6">
                  <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-lg bg-primary/10">
                    <ShieldCheck className="h-6 w-6 text-primary" />
                  </div>
                  <div>
                    <h3 className="mb-1 font-semibold text-card-foreground">Доверие и репутация</h3>
                    <p className="text-sm text-muted-foreground">
                      Верифицированные отзывы повышают доверие. В будущем — «Золотой список».
                    </p>
                  </div>
                </CardContent>
              </Card>
            </div>

            <div className="flex justify-center">
              <Button
                size="lg"
                className="gap-2 px-10"
                onClick={() => setStep(isMasterTrack ? 3 : 2)}
              >
                Подать заявку
                <ArrowRight className="h-4 w-4" />
              </Button>
            </div>

            <p className="mt-4 text-center text-sm text-muted-foreground">
              Регистрация бесплатная. Без скрытых платежей.
            </p>
            <p className="mt-3 text-center text-sm text-muted-foreground">
              {isMasterTrack ? (
                <>
                  Владелец сети?{" "}
                  <Link href="/partner" className="font-medium text-primary hover:underline">
                    Оформление для владельца
                  </Link>
                </>
              ) : (
                <>
                  Вы пар-мастер?{" "}
                  <Link
                    href="/partner/master"
                    className="font-medium text-primary hover:underline"
                  >
                    Подать заявку
                  </Link>
                </>
              )}
            </p>
          </div>
        )}

        {step === 3 && !isMasterTrack && (
          <PartnerVenueRegistrationCard
            values={venueFields}
            onChange={patchVenue}
            footer={
              <>
                {error && <p className="text-sm text-destructive">{error}</p>}
                <div className="flex gap-3 pt-2">
                  <Button variant="outline" className="gap-2" onClick={() => setStep(2)}>
                    <ArrowLeft className="h-4 w-4" />
                    Назад
                  </Button>
                  <Button
                    className="flex-1 gap-2"
                    onClick={handleSubmit}
                    disabled={loading || !partnerVenueStepCanProceed(venueFields)}
                  >
                    {loading ? "Создаём аккаунт и заведение..." : "Завершить регистрацию"}
                    {!loading && <CheckCircle2 className="h-4 w-4" />}
                  </Button>
                </div>
              </>
            }
            caption="Шаг 3 из 3 · Карточка будет создана как черновик; на модерацию вы отправите её после заполнения"
          />
        )}

        {/* Contacts & Account — step 2 for venue track, step 3 for master track */}
        {((!isMasterTrack && step === 2) || (isMasterTrack && step === 3)) && (
          <div className="mx-auto max-w-lg">
            <Card className="border-border">
              <CardHeader className="text-center">
                <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
                  <Users className="h-6 w-6 text-primary" />
                </div>
                <CardTitle className="text-2xl text-card-foreground">Контактные данные</CardTitle>
                <CardDescription>
                  {isMasterTrack
                    ? "Создадим аккаунт пар-мастера. Телефон укажите здесь — он подставится в карточку профиля, повторно вводить не нужно."
                    : "Создадим аккаунт владельца и свяжемся, если будут вопросы"}
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="space-y-2">
                  <Label htmlFor="contactName">Ваше имя</Label>
                  <Input
                    id="contactName"
                    placeholder="Иван Иванов"
                    value={contactName}
                    onChange={(e) => setContactName(e.target.value)}
                    required
                  />
                </div>

                <div className="space-y-2">
                  <Label htmlFor="contactEmail">Email</Label>
                  <Input
                    id="contactEmail"
                    type="email"
                    placeholder="ivan@banya.ru"
                    value={contactEmail}
                    onChange={(e) => setContactEmail(e.target.value)}
                    required
                  />
                </div>

                <div className="space-y-2">
                  <Label htmlFor="contactPhone">Телефон</Label>
                  <PhoneInput
                    id="contactPhone"
                    value={contactPhone}
                    onChange={setContactPhone}
                    required
                  />
                  {isMasterTrack && (
                    <p className="text-xs text-muted-foreground">
                      Этот номер увидят клиенты. Он автоматически появится в карточке профиля.
                    </p>
                  )}
                </div>

                <div className="space-y-2">
                  <Label htmlFor="contactPassword">Придумайте пароль</Label>
                  <Input
                    id="contactPassword"
                    type="password"
                    placeholder={`Минимум ${PASSWORD_MIN_LENGTH} символов`}
                    value={contactPassword}
                    onChange={(e) => setContactPassword(e.target.value)}
                    required
                    minLength={PASSWORD_MIN_LENGTH}
                    aria-invalid={contactPassword.length > 0 && !passwordValid}
                  />
                  <PasswordStrength value={contactPassword} />
                </div>

                <ConsentCheckbox checked={consent} onCheckedChange={setConsent} />
                <p className="text-xs text-muted-foreground">
                  Регистрируясь, вы принимаете{" "}
                  <Link
                    href={isMasterTrack ? "/legal/master-agreement" : "/legal/venue-agreement"}
                    target="_blank"
                    className="text-primary underline"
                  >
                    договор-оферту {isMasterTrack ? "пар-мастера" : "заведения"}
                  </Link>
                  .
                </p>

                {error && <p className="text-sm text-destructive">{error}</p>}

                <div className="flex gap-3 pt-2">
                  <Button
                    variant="outline"
                    className="gap-2"
                    onClick={() => setStep(1)}
                  >
                    <ArrowLeft className="h-4 w-4" />
                    Назад
                  </Button>
                  {isMasterTrack ? (
                    <Button
                      className="flex-1 gap-2"
                      onClick={handleSubmit}
                      disabled={
                        loading ||
                        !contactName ||
                        !contactEmail ||
                        !passwordValid ||
                        !getRawPhone(contactPhone).trim() ||
                        !consent
                      }
                    >
                      {loading ? "Регистрация..." : "Зарегистрироваться"}
                      {!loading && <CheckCircle2 className="h-4 w-4" />}
                    </Button>
                  ) : (
                    <Button
                      className="flex-1 gap-2"
                      onClick={() => setStep(3)}
                      disabled={
                        !contactName ||
                        !contactEmail ||
                        !passwordValid ||
                        !getRawPhone(contactPhone).trim() ||
                        !consent
                      }
                    >
                      Далее
                      <ArrowRight className="h-4 w-4" />
                    </Button>
                  )}
                </div>
              </CardContent>
            </Card>

            <p className="mt-4 text-center text-sm text-muted-foreground">
              {isMasterTrack
                ? "Шаг 2 из 2 · Далее откроется кабинет для заполнения профиля мастера"
                : "Шаг 2 из 3 · Все данные можно изменить позже"}
            </p>
          </div>
        )}

        {/* Step 4: Success */}
        {step === 4 && (
          <div className="mx-auto max-w-lg">
            <Card className="border-border">
              <CardContent className="py-10 text-center">
                <div className="mx-auto mb-6 flex h-16 w-16 items-center justify-center rounded-full bg-green-100">
                  <PartyPopper className="h-8 w-8 text-green-600" />
                </div>
                <h2 className="mb-2 text-2xl font-bold text-card-foreground">
                  Добро пожаловать в БаняГид!
                </h2>
                <p className="mb-6 text-muted-foreground">
                  Аккаунт создан. Карточка «{venueFields.venueName}» сохранена как{" "}
                  <strong>черновик</strong>: модератор её не видит, пока вы не
                  заполните витрину и не отправите заявку на проверку из
                  редактирования карточки.
                </p>

                <div className="mb-6 rounded-lg bg-secondary/50 p-4 text-left">
                  <div className="flex items-center gap-2 mb-2">
                    <Building2 className="h-4 w-4 text-primary" />
                    <span className="font-medium text-card-foreground">{venueFields.venueName}</span>
                    <Badge variant="secondary" className="text-xs">
                      {venueTypeBadgeLabel(venueFields.venueType)}
                    </Badge>
                  </div>
                  <p className="text-sm text-muted-foreground">
                    {venueFields.venueCity}, {venueFields.venueAddress}
                  </p>
                </div>

                <div className="flex flex-col gap-3 sm:flex-row">
                  <Button asChild className="flex-1 gap-2" size="lg">
                    <Link
                      href={
                        createdVenueId
                          ? `/owner/venues/${createdVenueId}/edit`
                          : "/owner/venues"
                      }
                    >
                      <Settings className="h-4 w-4" />
                      Заполнить карточку
                    </Link>
                  </Button>
                  <Button asChild variant="outline" className="flex-1 gap-2">
                    <Link href="/owner/venues">
                      <Building2 className="h-4 w-4" />
                      Все мои заведения
                    </Link>
                  </Button>
                </div>
                <p className="mt-4 text-center text-xs text-muted-foreground">
                  Публичная страница появится после публикации; в черновике она
                  недоступна по ссылке для гостей.
                </p>
              </CardContent>
            </Card>
          </div>
        )}

        {/* Step 5: Confirm email before the venue draft can be created */}
        {step === 5 && (
          <div className="mx-auto max-w-lg">
            <Card className="border-border">
              <CardHeader className="text-center">
                <CardTitle className="text-2xl">Остался один шаг</CardTitle>
                <CardDescription>
                  Аккаунт создан. Чтобы сохранить карточку «{venueFields.venueName}»,
                  подтвердите email — так мы защищаем каталог от поддельных заявок.
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <EmailVerificationNotice email={contactEmail} />
                {error ? <p className="text-sm text-destructive">{error}</p> : null}
                <Button
                  className="w-full gap-2"
                  size="lg"
                  disabled={loading}
                  onClick={handleVenueRetryAfterVerify}
                >
                  {loading ? "Проверяем…" : "Я подтвердил email — продолжить"}
                </Button>
                <p className="text-center text-xs text-muted-foreground">
                  Откройте письмо в другой вкладке, перейдите по ссылке, затем вернитесь
                  сюда и нажмите кнопку. Введённые данные карточки сохранены.
                </p>
              </CardContent>
            </Card>
          </div>
        )}
      </div>
    </div>
  )
}

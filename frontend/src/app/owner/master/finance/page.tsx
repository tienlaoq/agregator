"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useForm, Controller } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  formatApiErrorMessage,
  getMyMasterProfile,
  getMasterBalance,
  getMasterPayoutMethod,
  listMasterLedger,
  listMasterPayouts,
  setMasterPayoutMethod,
} from "@/lib/api";
import { useAuthStore } from "@/store/auth";
import { SbpBankSelect } from "@/components/banya/sbp-bank-select";
import type { SetPayoutMethodInput } from "@/lib/types";
import { LEDGER_ENTRY_LABELS, PAYOUT_STATUS_LABELS } from "@/lib/types";
import {
  Banknote,
  Clock,
  Landmark,
  Loader2,
  Smartphone,
  Wallet,
} from "lucide-react";

/** Копейки → строка вида «1 234,50 ₽». */
function formatKopecks(kopecks: number): string {
  return (kopecks / 100).toLocaleString("ru-RU", {
    style: "currency",
    currency: "RUB",
    minimumFractionDigits: 2,
  });
}

function formatDt(iso: string | undefined): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString("ru-RU", {
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

const bankAccountSchema = z.object({
  bank_bic: z
    .string()
    .transform((s) => s.replace(/\D/g, ""))
    .refine((s) => s.length === 9, "БИК должен содержать 9 цифр"),
  bank_account: z
    .string()
    .transform((s) => s.replace(/\D/g, ""))
    .refine((s) => s.length === 20, "Расчётный счёт должен содержать 20 цифр"),
  bank_name: z.string().trim().min(1, "Укажите название банка"),
  recipient_inn: z
    .string()
    .transform((s) => s.replace(/\D/g, ""))
    .refine(
      (s) => s.length === 10 || s.length === 12,
      "ИНН должен содержать 10 цифр (юрлицо) или 12 (ИП / физлицо)",
    ),
  recipient_kpp: z
    .string()
    .transform((s) => s.replace(/\D/g, ""))
    .refine((s) => s.length === 0 || s.length === 9, "КПП должен содержать 9 цифр")
    .optional(),
  recipient_name: z.string().trim().min(1, "Укажите получателя платежа"),
});

const sbpSchema = z.object({
  sbp_phone: z
    .string()
    .transform((s) => s.replace(/[^\d+]/g, ""))
    .refine(
      (s) => /^(\+7|7|8)\d{10}$/.test(s),
      "Телефон в формате +7XXXXXXXXXX",
    ),
  sbp_bank_id: z.string().trim().min(1, "Выберите банк"),
});

type BankAccountForm = z.input<typeof bankAccountSchema>;
type SbpForm = z.input<typeof sbpSchema>;

const PAYOUT_STATUS_BADGE: Record<string, "default" | "secondary" | "destructive" | "outline"> = {
  pending: "secondary",
  processing: "secondary",
  succeeded: "default",
  failed: "destructive",
};

export default function MasterFinancePage() {
  const router = useRouter();
  const { token, user, hydrated } = useAuthStore();
  const queryClient = useQueryClient();

  useEffect(() => {
    if (hydrated && (!token || user?.role !== "master")) {
      router.push("/auth/login");
    }
  }, [hydrated, token, user, router]);

  const enabled = !!token && user?.role === "master";

  const { data: profile } = useQuery({
    queryKey: ["my-master-profile"],
    queryFn: getMyMasterProfile,
    enabled,
    retry: false,
  });

  const balanceQ = useQuery({
    queryKey: ["master-balance"],
    queryFn: getMasterBalance,
    enabled,
  });
  const methodQ = useQuery({
    queryKey: ["master-payout-method"],
    queryFn: getMasterPayoutMethod,
    enabled,
  });
  const payoutsQ = useQuery({
    queryKey: ["master-payouts"],
    queryFn: () => listMasterPayouts({ limit: 50 }),
    enabled,
  });
  const ledgerQ = useQuery({
    queryKey: ["master-ledger"],
    queryFn: () => listMasterLedger({ limit: 50 }),
    enabled,
  });

  const saveMethodMu = useMutation({
    mutationFn: (input: SetPayoutMethodInput) => setMasterPayoutMethod(input),
    onSuccess: () => {
      toast.success("Реквизиты сохранены. Выплаты будут отправляться автоматически.");
      queryClient.invalidateQueries({ queryKey: ["master-payout-method"] });
    },
    onError: (e) => {
      toast.error(formatApiErrorMessage(e, "Не удалось сохранить реквизиты"));
    },
  });

  const bankForm = useForm<BankAccountForm>({
    resolver: zodResolver(bankAccountSchema),
    defaultValues: {
      bank_bic: "",
      bank_account: "",
      bank_name: "",
      recipient_inn: "",
      recipient_kpp: "",
      recipient_name: "",
    },
  });
  const sbpForm = useForm<SbpForm>({
    resolver: zodResolver(sbpSchema),
    defaultValues: { sbp_phone: "", sbp_bank_id: "" },
  });

  // Префилл из реквизитов профиля: получателя и ИНН мастер уже указывал в
  // разделе «Реквизиты для выплат» профиля — не заставляем вводить второй раз.
  // Только пока форма не тронута, чтобы не затирать ввод.
  const { reset: resetBankForm, getValues: getBankValues, formState: bankFormState } = bankForm;
  useEffect(() => {
    if (!profile || bankFormState.isDirty) return;
    const name = profile.payout_legal_name?.trim() ?? "";
    const inn = profile.payout_inn?.replace(/\D/g, "") ?? "";
    if (!name && !inn) return;
    resetBankForm({
      ...getBankValues(),
      recipient_name: name,
      recipient_inn: inn,
    });
  }, [profile, bankFormState.isDirty, resetBankForm, getBankValues]);

  if (!hydrated || !token || user?.role !== "master") return null;

  const method = methodQ.data;
  const balance = balanceQ.data;

  return (
    <div className="container mx-auto max-w-5xl px-4 py-8">
      <div className="mb-6">
        <h1 className="text-3xl font-bold">Финансы</h1>
        {profile ? (
          <p className="text-muted-foreground">{profile.display_name}</p>
        ) : null}
      </div>

      {/* Баланс */}
      <div className="mb-6 grid gap-4 sm:grid-cols-3">
        <Card>
          <CardHeader className="pb-2">
            <CardDescription className="flex items-center gap-1">
              <Wallet className="h-4 w-4" />
              Доступно к выплате
            </CardDescription>
          </CardHeader>
          <CardContent>
            {balanceQ.isPending ? (
              <Skeleton className="h-8 w-28" />
            ) : (
              <div className="text-2xl font-bold">
                {formatKopecks(balance?.available_kopecks ?? 0)}
              </div>
            )}
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardDescription className="flex items-center gap-1">
              <Clock className="h-4 w-4" />
              На удержании
            </CardDescription>
          </CardHeader>
          <CardContent>
            {balanceQ.isPending ? (
              <Skeleton className="h-8 w-28" />
            ) : (
              <div className="text-2xl font-bold">
                {formatKopecks(balance?.held_kopecks ?? 0)}
              </div>
            )}
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardDescription className="flex items-center gap-1">
              <Banknote className="h-4 w-4" />
              Всего на балансе
            </CardDescription>
          </CardHeader>
          <CardContent>
            {balanceQ.isPending ? (
              <Skeleton className="h-8 w-28" />
            ) : (
              <div className="text-2xl font-bold">
                {formatKopecks(balance?.total_kopecks ?? 0)}
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Реквизиты */}
      <Card className="mb-6">
        <CardHeader>
          <CardTitle>Реквизиты для выплат</CardTitle>
          <CardDescription>
            Выплаты отправляются автоматически, когда доступная сумма достигает
            100 ₽. Деньги за визит становятся доступны после его завершения.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          {methodQ.isPending ? (
            <Skeleton className="h-10 w-full" />
          ) : method ? (
            <div className="flex flex-wrap items-center gap-2 rounded-lg border bg-secondary/40 p-4">
              {method.kind === "bank_account" ? (
                <Landmark className="h-5 w-5 text-primary" />
              ) : (
                <Smartphone className="h-5 w-5 text-primary" />
              )}
              <div className="text-sm">
                <span className="font-medium">
                  {method.kind === "bank_account"
                    ? `Счёт ${method.bank_account_masked ?? ""} · ${method.bank_name ?? ""}`
                    : method.kind === "sbp"
                      ? `СБП ${method.sbp_phone_masked ?? ""}`
                      : `Карта •••• ${method.card_last4 ?? ""}`}
                </span>
                <span className="ml-2 text-muted-foreground">
                  обновлено {formatDt(method.updated_at)}
                </span>
              </div>
              <Badge variant="secondary" className="ml-auto">
                Активны
              </Badge>
            </div>
          ) : (
            <div className="rounded-lg border border-amber-300 bg-amber-50 p-4 text-sm text-amber-900 dark:border-amber-700 dark:bg-amber-950 dark:text-amber-200">
              Реквизиты не указаны — заработанные деньги копятся на балансе, но
              выплаты не отправляются. Заполните форму ниже.
            </div>
          )}

          <Tabs defaultValue={method?.kind === "sbp" ? "sbp" : "bank_account"}>
            <TabsList>
              <TabsTrigger value="bank_account" className="gap-1">
                <Landmark className="h-4 w-4" />
                Расчётный счёт
              </TabsTrigger>
              <TabsTrigger value="sbp" className="gap-1">
                <Smartphone className="h-4 w-4" />
                СБП
              </TabsTrigger>
            </TabsList>

            <TabsContent value="bank_account" className="mt-4">
              <form
                className="grid gap-4 sm:grid-cols-2"
                onSubmit={bankForm.handleSubmit((values) => {
                  const parsed = bankAccountSchema.parse(values);
                  saveMethodMu.mutate({ kind: "bank_account", ...parsed });
                })}
              >
                <div className="space-y-1.5">
                  <Label htmlFor="recipient_name">Получатель</Label>
                  <Input
                    id="recipient_name"
                    placeholder="ИП Иванов И. И. или Иванов Иван Иванович"
                    {...bankForm.register("recipient_name")}
                  />
                  {bankForm.formState.errors.recipient_name ? (
                    <p className="text-sm text-destructive">
                      {bankForm.formState.errors.recipient_name.message}
                    </p>
                  ) : null}
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="recipient_inn">ИНН</Label>
                  <Input
                    id="recipient_inn"
                    inputMode="numeric"
                    placeholder="10 или 12 цифр"
                    {...bankForm.register("recipient_inn")}
                  />
                  {bankForm.formState.errors.recipient_inn ? (
                    <p className="text-sm text-destructive">
                      {bankForm.formState.errors.recipient_inn.message}
                    </p>
                  ) : null}
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="recipient_kpp">КПП (только для юрлиц)</Label>
                  <Input
                    id="recipient_kpp"
                    inputMode="numeric"
                    placeholder="9 цифр"
                    {...bankForm.register("recipient_kpp")}
                  />
                  {bankForm.formState.errors.recipient_kpp ? (
                    <p className="text-sm text-destructive">
                      {bankForm.formState.errors.recipient_kpp.message}
                    </p>
                  ) : null}
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="bank_name">Банк</Label>
                  <Input
                    id="bank_name"
                    placeholder="АО «Т-Банк»"
                    {...bankForm.register("bank_name")}
                  />
                  {bankForm.formState.errors.bank_name ? (
                    <p className="text-sm text-destructive">
                      {bankForm.formState.errors.bank_name.message}
                    </p>
                  ) : null}
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="bank_bic">БИК</Label>
                  <Input
                    id="bank_bic"
                    inputMode="numeric"
                    placeholder="9 цифр"
                    {...bankForm.register("bank_bic")}
                  />
                  {bankForm.formState.errors.bank_bic ? (
                    <p className="text-sm text-destructive">
                      {bankForm.formState.errors.bank_bic.message}
                    </p>
                  ) : null}
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="bank_account">Расчётный счёт</Label>
                  <Input
                    id="bank_account"
                    inputMode="numeric"
                    placeholder="20 цифр"
                    {...bankForm.register("bank_account")}
                  />
                  {bankForm.formState.errors.bank_account ? (
                    <p className="text-sm text-destructive">
                      {bankForm.formState.errors.bank_account.message}
                    </p>
                  ) : null}
                </div>
                <div className="sm:col-span-2">
                  <Button type="submit" disabled={saveMethodMu.isPending} className="gap-1">
                    {saveMethodMu.isPending ? (
                      <Loader2 className="h-4 w-4 animate-spin" />
                    ) : null}
                    Сохранить реквизиты
                  </Button>
                </div>
              </form>
            </TabsContent>

            <TabsContent value="sbp" className="mt-4">
              <form
                className="grid gap-4 sm:grid-cols-2"
                onSubmit={sbpForm.handleSubmit((values) => {
                  const parsed = sbpSchema.parse(values);
                  saveMethodMu.mutate({ kind: "sbp", ...parsed });
                })}
              >
                <div className="space-y-1.5">
                  <Label htmlFor="sbp_phone">Телефон, привязанный к СБП</Label>
                  <Input
                    id="sbp_phone"
                    inputMode="tel"
                    placeholder="+7 900 000-00-00"
                    {...sbpForm.register("sbp_phone")}
                  />
                  {sbpForm.formState.errors.sbp_phone ? (
                    <p className="text-sm text-destructive">
                      {sbpForm.formState.errors.sbp_phone.message}
                    </p>
                  ) : null}
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="sbp_bank_id">Банк</Label>
                  <Controller
                    control={sbpForm.control}
                    name="sbp_bank_id"
                    render={({ field }) => (
                      <SbpBankSelect
                        value={field.value}
                        onChange={field.onChange}
                      />
                    )}
                  />
                  {sbpForm.formState.errors.sbp_bank_id ? (
                    <p className="text-sm text-destructive">
                      {sbpForm.formState.errors.sbp_bank_id.message}
                    </p>
                  ) : null}
                </div>
                <div className="sm:col-span-2">
                  <Button type="submit" disabled={saveMethodMu.isPending} className="gap-1">
                    {saveMethodMu.isPending ? (
                      <Loader2 className="h-4 w-4 animate-spin" />
                    ) : null}
                    Сохранить реквизиты
                  </Button>
                </div>
              </form>
            </TabsContent>
          </Tabs>
        </CardContent>
      </Card>

      {/* История */}
      <Tabs defaultValue="payouts">
        <TabsList>
          <TabsTrigger value="payouts">Выплаты</TabsTrigger>
          <TabsTrigger value="ledger">Журнал начислений</TabsTrigger>
        </TabsList>

        <TabsContent value="payouts" className="mt-4">
          <Card>
            <CardContent className="pt-6">
              {payoutsQ.isPending ? (
                <Skeleton className="h-24 w-full" />
              ) : (payoutsQ.data?.length ?? 0) === 0 ? (
                <p className="py-6 text-center text-sm text-muted-foreground">
                  Выплат пока не было. Они появятся после первых завершённых
                  визитов.
                </p>
              ) : (
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Дата</TableHead>
                      <TableHead>Сумма</TableHead>
                      <TableHead>Куда</TableHead>
                      <TableHead>Статус</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {payoutsQ.data?.map((p) => (
                      <TableRow key={p.id}>
                        <TableCell>{formatDt(p.created_at)}</TableCell>
                        <TableCell className="font-medium">
                          {formatKopecks(p.amount_kopecks)}
                        </TableCell>
                        <TableCell className="text-muted-foreground">
                          {p.method_display}
                        </TableCell>
                        <TableCell>
                          <Badge variant={PAYOUT_STATUS_BADGE[p.status] ?? "outline"}>
                            {PAYOUT_STATUS_LABELS[p.status] ?? p.status}
                          </Badge>
                          {p.status === "failed" && p.failure_reason ? (
                            <p className="mt-1 text-xs text-muted-foreground">
                              {p.failure_reason}
                            </p>
                          ) : null}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="ledger" className="mt-4">
          <Card>
            <CardContent className="pt-6">
              {ledgerQ.isPending ? (
                <Skeleton className="h-24 w-full" />
              ) : (ledgerQ.data?.length ?? 0) === 0 ? (
                <p className="py-6 text-center text-sm text-muted-foreground">
                  Начислений пока нет. Они появляются после успешной оплаты
                  визита.
                </p>
              ) : (
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Дата</TableHead>
                      <TableHead>Операция</TableHead>
                      <TableHead>Сумма</TableHead>
                      <TableHead>Доступно с</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {ledgerQ.data?.map((e) => (
                      <TableRow key={e.id}>
                        <TableCell>{formatDt(e.created_at)}</TableCell>
                        <TableCell>
                          {LEDGER_ENTRY_LABELS[e.entry_type] ?? e.entry_type}
                          {e.reason ? (
                            <p className="text-xs text-muted-foreground">{e.reason}</p>
                          ) : null}
                        </TableCell>
                        <TableCell
                          className={
                            e.amount_kopecks < 0
                              ? "font-medium text-destructive"
                              : "font-medium text-green-700 dark:text-green-400"
                          }
                        >
                          {e.amount_kopecks > 0 ? "+" : ""}
                          {formatKopecks(e.amount_kopecks)}
                        </TableCell>
                        <TableCell className="text-muted-foreground">
                          {e.entry_type === "accrual" ? formatDt(e.available_at) : "—"}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              )}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}

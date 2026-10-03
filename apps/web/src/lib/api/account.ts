import { queryOptions } from "@tanstack/react-query";

import { accountClient } from "~/lib/rpc";

export const meQuery = () =>
	queryOptions({
		queryKey: ["account", "me"],
		queryFn: async () => (await accountClient.getMe({})).user,
	});

export const exportData = () => accountClient.exportData({});

export const deleteAccount = (password: string) =>
	accountClient.deleteAccount({ password });

import { Dispatch, SetStateAction } from "react";
import { useToast } from "src/hooks/Toast";
import * as GQL from "src/core/generated-graphql";
import {
  useGroupCreate,
  usePerformerCreate,
  useStudioCreate,
  useTagCreate,
} from "src/core/StashService";
import { ObjectScrapeResult, ScrapeResult } from "./scrapeResult";
import { useIntl } from "react-intl";
import { scrapedPerformerToCreateInput } from "src/core/performers";
import { scrapedGroupToCreateInput } from "src/core/groups";

function useCreateObject<T>(
  entityTypeID: string,
  createFunc: (o: T) => Promise<void>
) {
  const Toast = useToast();
  const intl = useIntl();

  async function createNewObject(objects: T | T[]) {
    try {
      for (const o of Array.isArray(objects) ? objects : [objects]) {
        await createFunc(o);

        Toast.success(
          intl.formatMessage(
            { id: "toast.created_entity" },
            {
              entity: intl
                .formatMessage({ id: entityTypeID })
                .toLocaleLowerCase(),
            }
          )
        );
      }
    } catch (e) {
      Toast.error(e);
    }
  }

  return createNewObject;
}

interface IUseCreateNewStudioProps {
  scrapeResult: ObjectScrapeResult<GQL.ScrapedStudio>;
  setScrapeResult: (
    scrapeResult: ObjectScrapeResult<GQL.ScrapedStudio>
  ) => void;
  setNewObject: (newObject: GQL.ScrapedStudio | undefined) => void;
  endpoint?: string;
}

export function useCreateScrapedStudio(props: IUseCreateNewStudioProps) {
  const [createStudio] = useStudioCreate();

  const { scrapeResult, setScrapeResult, setNewObject } = props;

  async function createNewStudio(toCreate: GQL.ScrapedStudio) {
    const input: GQL.StudioCreateInput = {
      name: toCreate.name,
      urls: toCreate.urls,
      aliases:
        toCreate.aliases
          ?.split(",")
          .map((a) => a.trim())
          .filter((a) => a) || [],
      details: toCreate.details,
      image: toCreate.image,
      tag_ids: (toCreate.tags ?? [])
        .filter((t) => t.stored_id)
        .map((t) => t.stored_id!),
    };

    if (props.endpoint && toCreate.remote_site_id) {
      input.stash_ids = [
        {
          endpoint: props.endpoint,
          stash_id: toCreate.remote_site_id,
        },
      ];
    }

    const result = await createStudio({
      variables: {
        input,
      },
    });

    // set the new studio as the value
    setScrapeResult(
      scrapeResult.cloneWithValue({
        stored_id: result.data!.studioCreate!.id,
        name: toCreate.name,
      })
    );
    setNewObject(undefined);
  }

  return useCreateObject("studio", createNewStudio);
}

interface IUseCreateNewObjectProps<T> {
  scrapeResult: ScrapeResult<T[]>;
  setScrapeResult: Dispatch<SetStateAction<ScrapeResult<T[]>>>;
  newObjects: T[];
  setNewObjects: Dispatch<SetStateAction<T[]>>;
  endpoint?: string;
}

export function useCreateScrapedPerformer(
  props: IUseCreateNewObjectProps<GQL.ScrapedPerformer>
) {
  const [createPerformer] = usePerformerCreate();

  const { setScrapeResult, setNewObjects } = props;

  async function createNewPerformer(toCreate: GQL.ScrapedPerformer) {
    const input = scrapedPerformerToCreateInput(toCreate, props.endpoint);

    const result = await createPerformer({
      variables: { input },
    });

    setScrapeResult((current) => {
      const newValue = [...(current.newValue ?? [])];
      if (result.data?.performerCreate)
        newValue.push({
          stored_id: result.data.performerCreate.id,
          name: result.data.performerCreate.name,
        });
      return current.cloneWithValue(newValue);
    });
    if (!props.newObjects.some((p) => p.name === toCreate.name))
      throw new Error("Could not find performer to remove");

    setNewObjects((current) => {
      const remaining = current.concat();
      const index = remaining.findIndex((p) => p.name === toCreate.name);
      if (index !== -1) remaining.splice(index, 1);
      return remaining;
    });
  }

  return useCreateObject("performer", createNewPerformer);
}

export function useCreateScrapedGroup(
  props: IUseCreateNewObjectProps<GQL.ScrapedGroup>
) {
  const { setScrapeResult, setNewObjects } = props;
  const [createGroup] = useGroupCreate();

  async function createNewGroup(toCreate: GQL.ScrapedGroup) {
    const input = scrapedGroupToCreateInput(toCreate);

    const result = await createGroup({
      variables: { input: input },
    });

    setScrapeResult((current) => {
      const newValue = [...(current.newValue ?? [])];
      if (result.data?.groupCreate)
        newValue.push({
          stored_id: result.data.groupCreate.id,
          name: result.data.groupCreate.name,
        });
      return current.cloneWithValue(newValue);
    });
    if (!props.newObjects.some((p) => p.name === toCreate.name))
      throw new Error("Could not find group to remove");

    setNewObjects((current) => {
      const remaining = current.concat();
      const index = remaining.findIndex((p) => p.name === toCreate.name);
      if (index !== -1) remaining.splice(index, 1);
      return remaining;
    });
  }

  return useCreateObject("group", createNewGroup);
}

export function useLinkScrapedTag(
  props: IUseCreateNewObjectProps<GQL.ScrapedTag>
) {
  const { setScrapeResult, setNewObjects } = props;

  function linkTag(id: string, matchedName: string, scrapedName: string) {
    setScrapeResult((current) =>
      current.cloneWithValue([
        ...(current.newValue ?? []),
        { stored_id: id, name: matchedName },
      ])
    );
    if (!props.newObjects.some((p) => p.name === scrapedName))
      throw new Error("Could not find tag to remove");

    setNewObjects((current) => {
      const remaining = current.concat();
      const index = remaining.findIndex((p) => p.name === scrapedName);
      if (index !== -1) remaining.splice(index, 1);
      return remaining;
    });
  }

  return linkTag;
}

export function useCreateScrapedTag(
  props: IUseCreateNewObjectProps<GQL.ScrapedTag>
) {
  const [createTag] = useTagCreate();
  const linkTag = useLinkScrapedTag(props);

  async function createNewTag(toCreate: GQL.ScrapedTag) {
    const input: GQL.TagCreateInput = {
      name: toCreate.name ?? "",
    };

    if (props.endpoint && toCreate.remote_site_id) {
      input.stash_ids = [
        {
          endpoint: props.endpoint,
          stash_id: toCreate.remote_site_id,
        },
      ];
    }

    const result = await createTag({
      variables: { input },
    });

    if (result.data?.tagCreate)
      linkTag(
        result.data.tagCreate.id,
        result.data.tagCreate.name,
        toCreate.name ?? ""
      );
  }

  return useCreateObject("tag", createNewTag);
}

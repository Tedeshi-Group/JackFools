// td2-parser.js — classifies incoming WebSocket messages into TD2 game events.
// Exported via window.__JF_TD2_PARSER__ for use by content.js.
(function () {
  "use strict";

  const DEATH_ROOM_RE = /умрёт|смерти|комнату смерти/i;

  /**
   * Classify a raw WebSocket JSON string into a structured game event.
   * @param {string} raw - Raw JSON string from WebSocket.
   * @returns {object|null} Parsed event or null if unrecognised.
   */
  function classify(raw) {
    let msg;
    try {
      msg = JSON.parse(raw);
    } catch (_) {
      return null;
    }

    if (!msg || typeof msg !== "object") return null;

    const opcode = msg.opcode;
    const result = msg.result;

    // --- audiencePlayer events (questions, votes, waiting) ---
    if (opcode === "object" && result && result.key === "audiencePlayer") {
      return classifyAudiencePlayer(result.val || {});
    }

    // --- textDescriptions events (correct answers, death room info, game events) ---
    if (opcode === "object" && result && result.key === "textDescriptions") {
      return classifyTextDescriptions(result.val || {});
    }

    // --- game credits / artifact ---
    if (opcode === "artifact") {
      return { type: "game_credits", artifactId: msg.artifactId, categoryId: msg.categoryId };
    }

    return null;
  }

  function classifyAudiencePlayer(val) {
    const kind = val.kind;
    const prompt = val.prompt || "";
    const choices = extractChoices(val.choices);
    const countGroupKey = val.countGroupKey || "";
    const hasSubmit = !!val.hasSubmit;
    const roundType = val.roundType || "";

    if (kind === "waiting") {
      return { type: "voting_closed" };
    }

    if (kind === "choices") {
      // Death room vote — detected by prompt keywords
      if (DEATH_ROOM_RE.test(prompt)) {
        return {
          type: "death_room_vote",
          prompt: prompt,
          choices: choices,
          countGroupKey: countGroupKey,
        };
      }

      // Final round
      if (roundType === "FinalRound") {
        return {
          type: "final_round_question",
          prompt: prompt,
          choices: choices,
          countGroupKey: countGroupKey,
          hasSubmit: hasSubmit,
        };
      }

      // Regular question
      return {
        type: "regular_question",
        prompt: prompt,
        choices: choices,
        countGroupKey: countGroupKey,
        hasSubmit: hasSubmit,
      };
    }

    return null;
  }

  function classifyTextDescriptions(val) {
    const descs = val.latestDescriptions;
    if (!Array.isArray(descs) || descs.length === 0) return null;

    const events = [];
    for (const d of descs) {
      const cat = d.category;
      const text = d.text || "";

      if (cat === "TEXT_DESCRIPTION_CORRECT_ANSWER") {
        events.push({
          type: "correct_answer",
          text: text,
          answerTexts: parseSingularAnswer(text),
        });
      } else if (cat === "TEXT_DESCRIPTION_CORRECT_ANSWERS") {
        events.push({
          type: "correct_answers",
          text: text,
          answerTexts: parsePluralAnswers(text),
        });
      } else if (cat === "TEXT_DESCRIPTION_KILLING_FLOOR_PLAYERS") {
        events.push({ type: "death_room_announced", text: text });
      } else if (cat === "TEXT_DESCRIPTION_KILLING_FLOOR_PLAYER_KILLED") {
        events.push({ type: "death_room_result", text: text });
      } else if (cat === "TEXT_DESCRIPTION_QUESTION_CORRECT_PLAYER" || cat === "TEXT_DESCRIPTION_QUESTION_CORRECT_PLAYERS") {
        events.push({ type: "correct_player", text: text });
      } else if (cat === "TEXT_DESCRIPTION_FINAL_ROUND_LEAD_SWAPPED_PLAYER") {
        events.push({ type: "final_lead_swap", text: text });
      } else if (cat === "TEXT_DESCRIPTION_FINAL_ROUND_LEAD_PLAYER") {
        events.push({ type: "final_lead", text: text });
      } else if (cat === "TEXT_DESCRIPTION_FINAL_ROUND_DEVOURED_PLAYER") {
        events.push({ type: "final_devoured", text: text });
      } else if (cat === "TEXT_DESCRIPTION_FINAL_ROUND_DEVOURED_AUDIENCE") {
        events.push({ type: "final_audience_devoured", text: text });
      } else if (cat === "TEXT_DESCRIPTION_FINAL_ROUND_BLOCKED_PLAYER") {
        events.push({ type: "final_blocked", text: text });
      } else if (cat === "TEXT_DESCRIPTION_FINAL_ROUND_ESCAPED_PLAYER") {
        events.push({ type: "final_escaped", text: text });
      } else if (cat === "TEXT_DESCRIPTION_END_GAME_CAUSE_OF_DEATH_PLAYER") {
        events.push({ type: "end_game_death", text: text });
      } else if (cat === "TEXT_DESCRIPTION_END_GAME_SURVIVOR_PLAYER") {
        events.push({ type: "end_game_survivor", text: text });
      }
    }

    if (events.length === 0) return null;
    if (events.length === 1) return events[0];
    return events;
  }

  function extractChoices(choicesRaw) {
    if (!Array.isArray(choicesRaw)) return [];
    const result = [];
    for (const c of choicesRaw) {
      if (c && typeof c === "object" && typeof c.text === "string") {
        result.push(c.text);
      }
    }
    return result;
  }

  // "Верный ответ: 1817" → ["1817"]
  function parseSingularAnswer(text) {
    const prefix = "Верный ответ: ";
    const idx = text.indexOf(prefix);
    if (idx === -1) return [text];
    return [text.substring(idx + prefix.length)];
  }

  // "Верные ответы: Сигмовидная кишка и Аппендикс" → ["Сигмовидная кишка", "Аппендикс"]
  function parsePluralAnswers(text) {
    const prefix = "Верные ответы: ";
    const idx = text.indexOf(prefix);
    if (idx === -1) return [text];
    const rest = text.substring(idx + prefix.length);
    return rest.split(" и ").map(function (s) { return s.trim(); });
  }

  // Strip [i]...[/i] markup for matching, preserve for storage
  function stripMarkup(text) {
    return text.replace(/\[\/?i\]/g, "");
  }

  window.__JF_TD2_PARSER__ = {
    classify: classify,
    stripMarkup: stripMarkup,
  };
})();

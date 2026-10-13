CREATE TABLE `roster_assignment` (
  `id` INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `created_ts` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `duty_date` VARCHAR(10) NOT NULL,
  `user_id` INT NOT NULL,
  UNIQUE(`duty_date`,`user_id`)
);

-- Roster Tracker has no private notes: every note is readable by everyone.
UPDATE `note` SET `visibility` = 'PUBLIC' WHERE `visibility` <> 'PUBLIC';
